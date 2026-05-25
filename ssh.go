package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	maxSSHRetries  = 3
	sshFailureCode = 255
)

var sshOpts = []string{"-q", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}

// buildSSHCommand builds an SSH command string. The remote command is wrapped
// in single quotes for the local shell; any single quotes inside remoteCmd
// must be escaped as '\'' so the local shell delivers the original string
// verbatim to ssh.
func buildSSHCommand(host, remoteCmd string) string {
	opts := strings.Join(sshOpts, " ")
	return fmt.Sprintf("/usr/bin/ssh %s %s '%s'", opts, host, escapeForSingleQuotes(remoteCmd))
}

func shellQuote(s string) string {
	return "'" + escapeForSingleQuotes(s) + "'"
}

func escapeForSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func containsShellSpecialChars(s string) bool {
	specialChars := `'"$` + "`" + `\!#&*(){}[]|;<>?~`
	for _, c := range s {
		if strings.ContainsRune(specialChars, c) {
			return true
		}
	}
	return false
}

// scp copies src to dst. Either endpoint may be host-qualified
// ("host:/path") or local. If recursive is true, -r is added.
func (r *Runner) scp(src, dst string, recursive bool) error {
	opts := strings.Join(sshOpts, " ")
	flagStr := ""
	if recursive {
		flagStr = "-r "
	}
	cmdLine := fmt.Sprintf("/usr/bin/scp %s%s %s %s", flagStr, opts, src, dst)
	if code := r.run(cmdLine, cmdLine, os.Stdout, os.Stderr, true); code != 0 {
		return fmt.Errorf("scp %s -> %s %s", src, dst, formatExitError(code))
	}
	return nil
}

// remoteIsDir returns true if path is a directory on host.
func (r *Runner) remoteIsDir(host, path string) bool {
	_, ok := r.probe(host, "test -d "+shellQuote(path))
	return ok
}
