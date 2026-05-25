package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	aideHome = "/var/lib/aide"
	aideExe  = "/usr/sbin/aide"

	// AIDE exit codes (per aide(1) manpage):
	//   0     no differences
	//   1-7   bitflag combinations of: 1=added, 2=removed, 4=changed
	//   14-23 real errors (config, IO, memory, ...)
	//   8-13  undefined; we treat as failures conservatively
	aideStatusMax = 7
)

// errChecksFailed signals that one or more site checks failed. main exits 1
// silently — the per-site failure messages have already been printed.
var errChecksFailed = errors.New("checks failed")

type Config struct {
	Update           any    `json:"update"` // string or []string
	Check            string `json:"check"`
	ConfigPath       string `json:"config_path,omitempty"`
	PasswordlessSudo bool   `json:"passwordless_sudo,omitempty"`
}

type Site struct {
	Name   string
	Dir    string
	Config Config
}

type checkResult struct {
	site   string
	passed bool
	output string
}

func loadSite(name, dbDir string) (Site, error) {
	cfg, err := getConfig(name, dbDir)
	if err != nil {
		return Site{}, err
	}
	return Site{Name: name, Dir: filepath.Join(dbDir, name), Config: cfg}, nil
}

func getSites(dbDir string) ([]string, error) {
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return nil, err
	}
	var sites []string
	for _, entry := range entries {
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

// getConfig loads site-specific configuration or returns defaults.
func getConfig(site, dbDir string) (Config, error) {
	config := Config{
		Update: []string{
			fmt.Sprintf("sudo %s --update", aideExe),
			fmt.Sprintf("echo %s|base64 -d|sudo -S mv %s/aide.db.new.gz %s/aide.db.gz", placeholderSudoPWB64, aideHome, aideHome),
		},
		Check: fmt.Sprintf("sudo %s --check", aideExe),
	}

	configFile := filepath.Join(dbDir, site, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", configFile, err)
	}

	var siteConfig Config
	if err := json.Unmarshal(data, &siteConfig); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", configFile, err)
	}

	if siteConfig.Update != nil {
		config.Update = siteConfig.Update
	}
	if siteConfig.Check != "" {
		config.Check = siteConfig.Check
	}
	config.ConfigPath = siteConfig.ConfigPath
	config.PasswordlessSudo = siteConfig.PasswordlessSudo
	return config, nil
}

// getUpdateCommands converts config.Update to []string. Returns an error if
// the JSON shape is neither a string nor an array of strings.
func getUpdateCommands(config Config) ([]string, error) {
	switch v := config.Update.(type) {
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case []interface{}:
		cmds := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("update[%d] must be a string", i)
			}
			cmds = append(cmds, s)
		}
		return cmds, nil
	case nil:
		return nil, fmt.Errorf("update is nil")
	default:
		return nil, fmt.Errorf("update must be a string or array of strings, got %T", v)
	}
}

// formatExitError renders an exit code as a human-readable suffix.
func formatExitError(code int) string {
	switch code {
	case exitCodeCancelled:
		return "cancelled"
	case exitCodeSpawnErr:
		return "spawn error"
	default:
		return fmt.Sprintf("failed with exit status %d", code)
	}
}

// checkSite runs AIDE check on a site with retry on SSH connection failures.
// out is always non-nil — sequential callers pass os.Stdout, parallel callers
// pass a bytes.Buffer that gets re-emitted in completion order.
func checkSite(r *Runner, site Site, out io.Writer) bool {
	fprintq(out, "################################## Checking %s\n", site.Name)

	sshCmd := buildSSHCommand(site.Name, site.Config.Check)

	var exitCode int
	for retry := 0; retry < maxSSHRetries; retry++ {
		if r.ctx.Err() != nil {
			exitCode = exitCodeCancelled
			break
		}
		exitCode = r.run(sshCmd, sshCmd, out, out, true)
		if exitCode != sshFailureCode {
			break
		}
		fprintq(out, "SSH connection failed, retrying (%d/%d)...\n", retry+1, maxSSHRetries)
	}

	if exitCode != 0 {
		fprintq(out, "%s: %s\n", sshCmd, formatExitError(exitCode))
	}

	fprintq(out, "################################## Finished %s\n", site.Name)
	return exitCode == 0
}

// runParallelChecks runs checks on multiple sites concurrently with a
// progress indicator. Results are returned in the same order as sites.
func runParallelChecks(r *Runner, sites []Site, maxParallel int) []checkResult {
	var wg sync.WaitGroup
	results := make([]checkResult, len(sites))
	total := len(sites)

	var progressMu sync.Mutex
	completed := 0

	var sem chan struct{}
	if maxParallel > 0 {
		sem = make(chan struct{}, maxParallel)
	}

	for i, site := range sites {
		wg.Add(1)
		go func(idx int, s Site) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}

			var buf bytes.Buffer
			passed := checkSite(r, s, &buf)

			results[idx] = checkResult{
				site:   s.Name,
				passed: passed,
				output: buf.String(),
			}

			progressMu.Lock()
			completed++
			status := "PASS"
			if !passed {
				status = "FAIL"
			}
			printq("\r[%d/%d] %s: %s\033[K\n", completed, total, s.Name, status)
			progressMu.Unlock()
		}(i, site)
	}

	wg.Wait()
	return results
}

// getMessage formats the results summary.
func getMessage(passed, failed []string) string {
	if len(passed) == 0 && len(failed) == 0 {
		return "No sites checked.\n\n"
	}
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

// runChecks runs AIDE checks across the requested sites.
func runChecks(r *Runner, args *cliArgs) error {
	names, err := getSites(args.dbDir)
	if err != nil {
		return fmt.Errorf("read sites: %w", err)
	}

	if len(args.positional) > 0 {
		names, err = filterSites(names, args.positional)
		if err != nil {
			return err
		}
	}

	if len(names) == 0 {
		printq("No sites to check.\n")
		return nil
	}

	// Load each site's config up front so we fail fast on parse errors.
	sites := make([]Site, 0, len(names))
	for _, name := range names {
		s, err := loadSite(name, args.dbDir)
		if err != nil {
			return fmt.Errorf("load site %s: %w", name, err)
		}
		sites = append(sites, s)
	}

	printq("Checking sites: %s\n", strings.Join(names, ", "))

	var passed, failed []string
	if args.parallel == 1 || verbose {
		// Sequential — verbose mode shows commands as they execute.
		for _, s := range sites {
			if r.ctx.Err() != nil {
				break
			}
			if checkSite(r, s, os.Stdout) {
				passed = append(passed, s.Name)
			} else {
				failed = append(failed, s.Name)
			}
		}
	} else {
		results := runParallelChecks(r, sites, args.parallel)
		for _, res := range results {
			printq("%s", res.output)
			if res.passed {
				passed = append(passed, res.site)
			} else {
				failed = append(failed, res.site)
			}
		}
	}

	msg := getMessage(passed, failed)
	if args.mail != "" {
		if err := sendMail(msg, args.mail); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send mail: %v\n", err)
		}
	}
	printq("%s", msg)

	if len(failed) > 0 {
		return errChecksFailed
	}
	return nil
}

// filterSites returns the intersection of available and requested site names,
// preserving the order of available. Unknown names produce an error.
func filterSites(available, requested []string) ([]string, error) {
	avail := make(map[string]bool, len(available))
	for _, name := range available {
		avail[name] = true
	}
	var unknown []string
	keep := make(map[string]bool, len(requested))
	for _, name := range requested {
		if !avail[name] {
			unknown = append(unknown, name)
		}
		keep[name] = true
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown site(s): %s", strings.Join(unknown, ", "))
	}
	var filtered []string
	for _, name := range available {
		if keep[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered, nil
}
