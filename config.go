package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type rcEntry struct {
	value  string
	source string
}

// rcLoaded records the rc files actually read, in load order
// (system first, user second). Populated by loadAidedConf.
var rcLoaded []string

// rcSearched records every path checked for an rc file, in search order,
// regardless of whether it existed. Populated by loadAidedConf.
var rcSearched []string

// rcSettings maps each key found in any rc file to its value and the
// path of the file it was last set in. The user file's entries overwrite
// the system file's. Populated by loadAidedConf.
var rcSettings = map[string]rcEntry{}

// loadAidedConf reads /etc/aided.conf then ~/.aided.conf, merging them.
// Missing files are not an error. Lines starting with # (after trimming
// whitespace) and blank lines are ignored. Each remaining line must be
// key=value; anything else is a fatal parse error. The returned map is
// keyed by flag name; per-key source paths are recorded in rcSettings.
func loadAidedConf() (map[string]string, error) {
	rcLoaded = nil
	rcSearched = nil
	rcSettings = map[string]rcEntry{}

	rcSearched = append(rcSearched, "/etc/aided.conf")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		rcSearched = append(rcSearched, filepath.Join(home, ".aided.conf"))
	}
	for _, p := range rcSearched {
		found, err := readAidedConf(p, rcSettings)
		if err != nil {
			return nil, err
		}
		if found {
			rcLoaded = append(rcLoaded, p)
		}
	}
	out := make(map[string]string, len(rcSettings))
	for k, e := range rcSettings {
		out[k] = e.value
	}
	return out, nil
}

func readAidedConf(path string, into map[string]rcEntry) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return true, fmt.Errorf("parse %s:%d: expected key=value", path, lineNo)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		into[key] = rcEntry{value: val, source: path}
	}
	if err := sc.Err(); err != nil {
		return true, fmt.Errorf("read %s: %w", path, err)
	}
	return true, nil
}
