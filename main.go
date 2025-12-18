package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	nRetry   = 3
	aideHome = "/var/lib/aide"
	aideExe  = "/usr/sbin/aide"
)

var (
	verbose bool
	sshOpts = []string{"-q", "-o", "StrictHostKeyChecking no", "-o", "UserKnownHostsFile /dev/null"}
)

// Config represents site-specific configuration
type Config struct {
	Update interface{} `json:"update"` // Can be string or []string
	Check  string      `json:"check"`
}

// checkResult holds the result of a site check
type checkResult struct {
	site   string
	passed bool
	output string
}

func main() {
	// Parse command-line arguments
	var dbDir string
	var update string
	var mail string
	var parallel int

	// Get default dbDir relative to executable
	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = os.Getwd()
	}
	defaultDbDir := filepath.Join(filepath.Dir(exePath), "db")

	// If default doesn't exist, try current directory
	if _, err := os.Stat(defaultDbDir); os.IsNotExist(err) {
		defaultDbDir = filepath.Join(".", "db")
	}

	flag.StringVar(&dbDir, "db", defaultDbDir, "database directory")
	flag.StringVar(&update, "update", "", "site to update")
	flag.StringVar(&mail, "mail", "", "email address for notifications")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose output")
	flag.IntVar(&parallel, "parallel", 1, "number of parallel checks (0 = unlimited)")
	flag.Parse()

	// Update SSH options for verbose mode
	if verbose {
		sshOpts = []string{"-v", "-o", "StrictHostKeyChecking no", "-o", "UserKnownHostsFile /dev/null"}
	}

	who := os.Getenv("USER")

	if update != "" {
		if who == "" {
			fmt.Fprintln(os.Stderr, "--who required (or set USER env var)")
			os.Exit(1)
		}
		if err := updateSite(update, dbDir, who); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Get list of sites from db directory
	sites, err := getSites(dbDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading sites: %v\n", err)
		os.Exit(1)
	}

	// Filter sites if specific ones provided on command line
	if flag.NArg() > 0 {
		selected := make(map[string]bool)
		for _, arg := range flag.Args() {
			selected[arg] = true
		}
		var filtered []string
		for _, site := range sites {
			if selected[site] {
				filtered = append(filtered, site)
			}
		}
		sites = filtered
	}

	var passed, failed []string

	if parallel == 1 {
		// Sequential execution (original behavior)
		for _, site := range sites {
			if checkSite(site, dbDir, nil) {
				passed = append(passed, site)
			} else {
				failed = append(failed, site)
			}
		}
	} else {
		// Parallel execution
		results := runParallelChecks(sites, dbDir, parallel)
		for _, r := range results {
			fmt.Print(r.output)
			if r.passed {
				passed = append(passed, r.site)
			} else {
				failed = append(failed, r.site)
			}
		}
	}

	msg := getMessage(passed, failed)

	if mail != "" {
		sendMail(msg, mail)
	}

	fmt.Print(msg)
}

// runParallelChecks runs checks on multiple sites concurrently
func runParallelChecks(sites []string, dbDir string, maxParallel int) []checkResult {
	var wg sync.WaitGroup
	results := make([]checkResult, len(sites))

	// Create semaphore for limiting concurrency
	var sem chan struct{}
	if maxParallel > 0 {
		sem = make(chan struct{}, maxParallel)
	}

	for i, site := range sites {
		wg.Add(1)
		go func(idx int, s string) {
			defer wg.Done()

			// Acquire semaphore if limited
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}

			// Capture output for this site
			var buf bytes.Buffer
			passed := checkSite(s, dbDir, &buf)

			results[idx] = checkResult{
				site:   s,
				passed: passed,
				output: buf.String(),
			}
		}(i, site)
	}

	wg.Wait()
	return results
}

// getSites returns a sorted list of site directories in dbDir
func getSites(dbDir string) ([]string, error) {
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return nil, err
	}

	var sites []string
	for _, entry := range entries {
		// Skip hidden files/directories (starting with .)
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			sites = append(sites, entry.Name())
		}
	}

	sort.Strings(sites)
	return sites, nil
}

// getConfig loads site-specific configuration or returns defaults
func getConfig(site, dbDir string) Config {
	config := Config{
		Update: []string{
			fmt.Sprintf("sudo %s --update", aideExe),
			fmt.Sprintf("echo {SUDO_PW}|sudo -S mv %s/aide.db.new.gz %s/aide.db.gz", aideHome, aideHome),
		},
		Check: fmt.Sprintf("sudo %s --check", aideExe),
	}

	configFile := filepath.Join(dbDir, site, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return config
	}

	var siteConfig Config
	if err := json.Unmarshal(data, &siteConfig); err != nil {
		return config
	}

	// Merge site config with defaults
	if siteConfig.Update != nil {
		config.Update = siteConfig.Update
	}
	if siteConfig.Check != "" {
		config.Check = siteConfig.Check
	}

	return config
}

// getUpdateCommands converts config.Update to []string
func getUpdateCommands(config Config) []string {
	switch v := config.Update.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		var cmds []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				cmds = append(cmds, s)
			}
		}
		return cmds
	default:
		return nil
	}
}

// runRemote executes a command and returns success/failure
// If out is nil, output goes to stdout/stderr; otherwise it's captured
func runRemote(cmdLine string, ignoreErrors bool, out *bytes.Buffer) bool {
	if verbose {
		if out != nil {
			fmt.Fprintln(out, cmdLine)
		} else {
			fmt.Println(cmdLine)
		}
	}

	cmd := exec.Command("/bin/sh", "-c", cmdLine)
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err := cmd.Run()
	if err != nil && !ignoreErrors {
		errMsg := fmt.Sprintf("%s: failed %v\n", cmdLine, err)
		if out != nil {
			fmt.Fprint(out, errMsg)
		} else {
			fmt.Fprint(os.Stderr, errMsg)
		}
		return false
	}

	return true
}

// checkSite runs AIDE check on a site with retry logic
// If out is nil, output goes to stdout; otherwise it's captured
func checkSite(site, dbDir string, out *bytes.Buffer) bool {
	for retry := 0; retry < nRetry; retry++ {
		header := fmt.Sprintf("################################## Checking %s\n", site)
		footer := fmt.Sprintf("################################## Finished %s\n", site)

		if out != nil {
			fmt.Fprint(out, header)
		} else {
			fmt.Print(header)
		}

		result := doCheck(site, dbDir, out)

		if out != nil {
			fmt.Fprint(out, footer)
		} else {
			fmt.Print(footer)
		}

		if result {
			return true
		}

		// Sleep random time before retry (up to 60 seconds)
		if retry < nRetry-1 {
			sleepTime := time.Duration(rand.Float64()*60) * time.Second
			time.Sleep(sleepTime)
		}
	}

	return false
}

// doCheck performs the actual AIDE check
func doCheck(site, dbDir string, out *bytes.Buffer) bool {
	config := getConfig(site, dbDir)

	sshCmd := buildSSHCommand(site, config.Check)
	return runRemote(sshCmd, false, out)
}

// updateSite updates the AIDE database on a site
func updateSite(site, dbDir, who string) error {
	config := getConfig(site, dbDir)
	updateCmds := getUpdateCommands(config)

	fmt.Printf("Updating %s\n", site)

	// Check if we need sudo password
	needsPassword := false
	for _, cmd := range updateCmds {
		if strings.Contains(cmd, "{SUDO_PW}") {
			needsPassword = true
			break
		}
	}

	var password string
	if needsPassword {
		fmt.Print("Enter sudo password: ")

		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // newline after password input
		password = string(bytePassword)
	}

	// Execute each update command
	for i := range updateCmds {
		cmd := updateCmds[i]
		if password != "" {
			cmd = strings.ReplaceAll(cmd, "{SUDO_PW}", password)
		}

		sshCmd := buildSSHCommand(site, cmd)
		if !runRemote(sshCmd, true, nil) {
			return fmt.Errorf("command failed")
		}
	}

	return nil
}

// buildSSHCommand builds an SSH command string
func buildSSHCommand(site, remoteCmd string) string {
	opts := strings.Join(sshOpts, " ")
	return fmt.Sprintf("/usr/bin/ssh %s %s '%s'", opts, site, remoteCmd)
}

// getMessage formats the results message
func getMessage(passed, failed []string) string {
	passes := "NONE"
	fails := "NONE"

	if len(passed) > 0 {
		passes = strings.Join(passed, ", ")
	}
	if len(failed) > 0 {
		fails = strings.Join(failed, ", ")
	}

	return fmt.Sprintf("Passed AIDE checks: %s\nFailed: %s\n\n", passes, fails)
}

// sendMail sends email notification
func sendMail(msg, address string) {
	cmd := exec.Command("mail", "-s", "AIDE checks", address)
	cmd.Stdin = strings.NewReader(msg)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send mail: %v\n", err)
	}
}
