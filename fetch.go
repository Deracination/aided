package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fetchUnset = "\x00unset\x00"

// runFetch copies the remote aide configuration into db/<site>/. If cliPath
// is non-empty it overrides (and is persisted to) config_path; otherwise the
// existing config_path from config.json is used.
func runFetch(r *Runner, site Site, cliPath string) error {
	configFile := filepath.Join(site.Dir, "config.json")
	cfgMap, exists, err := readConfigMap(configFile)
	if err != nil {
		return err
	}
	if cfgMap == nil {
		cfgMap = make(map[string]any)
	}

	path := cliPath
	if path == "" {
		if !exists {
			return fmt.Errorf("no config.json for site %q — rerun with --fetch=<path> %s", site.Name, site.Name)
		}
		stored, _ := cfgMap["config_path"].(string)
		if stored == "" {
			return fmt.Errorf("config.json for %q has no config_path — rerun with --fetch=<path> %s", site.Name, site.Name)
		}
		path = stored
	} else {
		cfgMap["config_path"] = path
		if err := writeConfigMap(configFile, cfgMap); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	fmt.Printf("Fetching %s:%s\n", site.Name, path)

	if r.remoteIsDir(site.Name, path) {
		aideDir := filepath.Join(site.Dir, "aide")
		if err := os.RemoveAll(aideDir); err != nil {
			return fmt.Errorf("remove old %s: %w", aideDir, err)
		}
		if err := os.MkdirAll(aideDir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", aideDir, err)
		}
		// `scp -r host:/etc/aide/. <local>/` copies the contents into <local>/.
		src := strings.TrimRight(path, "/") + "/."
		if err := r.scp(site.Name+":"+src, aideDir, true); err != nil {
			return err
		}
		fmt.Printf("Wrote %s/\n", aideDir)
		return nil
	}

	if err := os.MkdirAll(site.Dir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(site.Dir, "aide.conf")
	if err := r.scp(site.Name+":"+path, dest, false); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", dest)
	return nil
}

// readConfigMap reads config.json as a generic map so unknown keys survive
// round-tripping. Returns (nil, false, nil) if the file does not exist.
func readConfigMap(path string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	m := make(map[string]any)
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, true, nil
}

func writeConfigMap(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// rewriteBareFetch turns a bare `--fetch`/`-fetch` token (no `=`) into
// `--fetch=`/`-fetch=` so the standard flag package treats fetch as a string
// flag whose value is optional. Tokens after `--` are left untouched.
func rewriteBareFetch(args *[]string) {
	for i, a := range *args {
		if a == "--" {
			return
		}
		if a == "--fetch" {
			(*args)[i] = "--fetch="
		} else if a == "-fetch" {
			(*args)[i] = "-fetch="
		}
	}
}
