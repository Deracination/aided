package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runInit initializes a new site configuration by probing the remote host.
func runInit(r *Runner, host, aidePath, configPath, dbDir string) error {
	fmt.Printf("Initializing site %s...\n", host)

	if aidePath == "" {
		fmt.Println("Searching for aide binary on remote host...")
		aidePath = r.findRemoteBinary(host, "aide", []string{
			"/usr/sbin/aide", "/usr/bin/aide", "/usr/local/bin/aide",
		})
		if aidePath == "" {
			aidePath = promptPath("aide binary not found. Enter path to aide executable: ")
			if aidePath == "" {
				return fmt.Errorf("aide binary path is required")
			}
		} else {
			fmt.Printf("Found aide at %s\n", aidePath)
		}
	}

	fmt.Println("Searching for aideinit on remote host...")
	aideinitPath := r.findRemoteBinary(host, "aideinit", []string{
		"/usr/sbin/aideinit", "/usr/local/bin/aideinit",
	})
	if aideinitPath != "" {
		fmt.Printf("Found aideinit at %s\n", aideinitPath)
	} else {
		fmt.Println("aideinit not found, will use aide --update fallback")
	}

	if configPath == "" {
		fmt.Println("Searching for aide config on remote host...")
		configPath = r.findRemoteFile(host, []string{
			"/etc/aide/aide.conf", "/etc/aide.conf", "/usr/local/etc/aide.conf",
		})
		if configPath == "" {
			configPath = promptPath("aide config not found. Enter path to aide.conf: ")
			if configPath == "" {
				return fmt.Errorf("aide config path is required")
			}
		} else {
			fmt.Printf("Found config at %s\n", configPath)
		}
	}

	checkCmd := fmt.Sprintf("sudo %s --config=%s --check", aidePath, configPath)
	var updateCmd any
	if aideinitPath != "" {
		updateCmd = fmt.Sprintf("sudo %s -y -f -- --config=%s", aideinitPath, configPath)
	} else {
		updateCmd = []string{
			fmt.Sprintf("sudo %s --config=%s --update", aidePath, configPath),
			fmt.Sprintf("echo %s|base64 -d|sudo -S mv %s/aide.db.new.gz %s/aide.db.gz", placeholderSudoPWB64, aideHome, aideHome),
		}
	}

	siteConfig := Config{Check: checkCmd, Update: updateCmd}

	siteDir := filepath.Join(dbDir, host)
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		return fmt.Errorf("create site directory: %w", err)
	}

	configJSON, err := json.MarshalIndent(siteConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configFile := filepath.Join(siteDir, "config.json")
	if err := os.WriteFile(configFile, configJSON, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("\nSite %s initialized successfully!\n", host)
	fmt.Printf("Config written to %s\n", configFile)
	return nil
}

// findRemoteBinary searches for an executable on host using `which name`
// first, then trying each of commonPaths with `test -x`.
func (r *Runner) findRemoteBinary(host, name string, commonPaths []string) string {
	if path, ok := r.probe(host, "which "+shellQuote(name)); ok && path != "" {
		return path
	}
	return r.findRemotePath(host, "-x", commonPaths)
}

// findRemoteFile checks each of commonPaths on host with `test -f`.
func (r *Runner) findRemoteFile(host string, commonPaths []string) string {
	return r.findRemotePath(host, "-f", commonPaths)
}

func (r *Runner) findRemotePath(host, testFlag string, commonPaths []string) string {
	for _, p := range commonPaths {
		if _, ok := r.probe(host, fmt.Sprintf("test %s %s", testFlag, shellQuote(p))); ok {
			return p
		}
	}
	return ""
}

func promptPath(prompt string) string {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}
