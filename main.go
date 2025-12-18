package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	aideHome = "/var/lib/aide"
	aideExe  = "/usr/sbin/aide"
)

var (
	verbose    bool
	sshVerbose bool
	quiet      bool
	sshOpts    = []string{"-q", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	logFile    *os.File
	ctx        context.Context
	cancel     context.CancelFunc
)

// printq prints to stdout unless quiet mode is enabled
func printq(format string, a ...any) {
	if !quiet {
		fmt.Printf(format, a...)
	}
}

// fprintq prints to a writer unless quiet mode is enabled
func fprintq(w io.Writer, format string, a ...any) {
	if !quiet {
		fmt.Fprintf(w, format, a...)
	}
}

// Config represents site-specific configuration
type Config struct {
	Update any    `json:"update"` // Can be string or []string
	Check  string `json:"check"`
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

	var logPath string

	flag.StringVar(&dbDir, "db", defaultDbDir, "database directory")
	flag.StringVar(&update, "u", "", "site to update")
	flag.StringVar(&mail, "m", "", "email address for notifications")
	flag.BoolVar(&verbose, "v", false, "print commands as they are executed")
	flag.BoolVar(&sshVerbose, "V", false, "enable SSH verbose output")
	flag.BoolVar(&quiet, "q", false, "quiet mode - suppress all output except errors")
	flag.IntVar(&parallel, "p", 0, "max parallel checks (0 = unlimited, 1 = sequential)")
	flag.StringVar(&logPath, "log", "./aided.log", "log file for SSH command output")
	flag.Parse()

	// Quiet mode overrides verbose
	if quiet {
		verbose = false
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel = context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		printq("\nReceived interrupt, terminating SSH commands...\n")
		cancel()
	}()

	// Open log file
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open log file %s: %v\n", logPath, err)
	} else {
		defer logFile.Close()
	}

	// Update SSH options for verbose mode
	if sshVerbose {
		sshOpts = []string{"-v", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	}

	if update != "" {
		if err := updateSite(update, dbDir); err != nil {
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

	// Print sites to be checked
	printq("Checking sites: %s\n", strings.Join(sites, ", "))

	var passed, failed []string

	if parallel == 1 || verbose {
		// Sequential execution (required for verbose to show commands in real-time)
		for _, site := range sites {
			if ctx.Err() != nil {
				break
			}
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
			printq("%s", r.output)
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

	printq("%s", msg)

	// Exit with code 1 if any checks failed
	if len(failed) > 0 {
		os.Exit(1)
	}
}

// runParallelChecks runs checks on multiple sites concurrently
func runParallelChecks(sites []string, dbDir string, maxParallel int) []checkResult {
	var wg sync.WaitGroup
	results := make([]checkResult, len(sites))
	total := len(sites)

	// Progress tracking
	var progressMu sync.Mutex
	completed := 0

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

			// Update progress
			progressMu.Lock()
			completed++
			status := "PASS"
			if !passed {
				status = "FAIL"
			}
			printq("\r[%d/%d] %s: %s\033[K\n", completed, total, s, status)
			progressMu.Unlock()
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

// runRemote executes a command and returns the exit code
// If out is nil, output goes to stdout/stderr; otherwise it's captured
// Returns 0 on success, or the exit code on failure
func runRemote(cmdLine string, out *bytes.Buffer) int {
	// Log command with timestamp
	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] %s\n", time.Now().Format(time.RFC3339), cmdLine)
	}

	if verbose {
		if out != nil {
			fmt.Fprintln(out, cmdLine)
		} else {
			fmt.Println(cmdLine)
		}
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdLine)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the entire process group to ensure child processes (ssh) are terminated
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Capture output for logging
	var logBuf bytes.Buffer
	if out != nil {
		if logFile != nil {
			cmd.Stdout = io.MultiWriter(out, &logBuf)
			cmd.Stderr = io.MultiWriter(out, &logBuf)
		} else {
			cmd.Stdout = out
			cmd.Stderr = out
		}
	} else {
		if logFile != nil {
			cmd.Stdout = io.MultiWriter(os.Stdout, &logBuf)
			cmd.Stderr = io.MultiWriter(os.Stderr, &logBuf)
		} else {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() != nil {
			// Context was cancelled (Ctrl+C)
			exitCode = -2
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Write captured output to log file
	if logFile != nil && logBuf.Len() > 0 {
		logFile.Write(logBuf.Bytes())
	}

	// Log exit code if non-zero
	if logFile != nil && exitCode != 0 {
		fmt.Fprintf(logFile, "[%s] Exit code: %d\n", time.Now().Format(time.RFC3339), exitCode)
	}

	return exitCode
}

const (
	maxSSHRetries  = 3
	sshFailureCode = 255
)

// checkSite runs AIDE check on a site with retry on SSH connection failures
// If out is nil, output goes to stdout so it is displayed immediately; otherwise it's captured
func checkSite(site, dbDir string, out *bytes.Buffer) bool {
	header := fmt.Sprintf("################################## Checking %s\n", site)
	footer := fmt.Sprintf("################################## Finished %s\n", site)

	if out != nil {
		fprintq(out, "%s", header)
	} else {
		printq("%s", header)
	}

	config := getConfig(site, dbDir)
	sshCmd := buildSSHCommand(site, config.Check)

	var exitCode int
	for retry := 0; retry < maxSSHRetries; retry++ {
		// Check if context was cancelled before attempting
		if ctx.Err() != nil {
			exitCode = -2
			break
		}
		exitCode = runRemote(sshCmd, out)
		if exitCode != sshFailureCode {
			break
		}
		// SSH connection failed, print retry message
		if out != nil {
			fprintq(out, "SSH connection failed, retrying (%d/%d)...\n", retry+1, maxSSHRetries)
		} else {
			printq("SSH connection failed, retrying (%d/%d)...\n", retry+1, maxSSHRetries)
		}
	}

	if exitCode != 0 {
		if out != nil {
			fprintq(out, "%s: failed exit status %d\n", sshCmd, exitCode)
		} else {
			fprintq(os.Stderr, "%s: failed exit status %d\n", sshCmd, exitCode)
		}
	}

	if out != nil {
		fprintq(out, "%s", footer)
	} else {
		printq("%s", footer)
	}

	return exitCode == 0
}

// updateSite updates the AIDE database on a site
func updateSite(site, dbDir string) error {
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
		if exitCode := runRemote(sshCmd, nil); exitCode != 0 {
			return fmt.Errorf("command failed with exit status %d", exitCode)
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
