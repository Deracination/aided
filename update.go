package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	placeholderSudoPW    = "{SUDO_PW}"
	placeholderSudoPWB64 = "{SUDO_PW_B64}"
	remoteTmpTemplate    = "/tmp/aided-%s"
)

// runUpdate updates the AIDE database on a site, optionally pushing the
// version-controlled aide.conf first. If noPassword is true (or the site
// has passwordless_sudo set in its config), the password prompt is
// skipped and sudo invocations are rewritten to use -n.
func runUpdate(r *Runner, site Site, noPassword bool) error {
	updateCmds, err := getUpdateCommands(site.Config)
	if err != nil {
		return fmt.Errorf("update config: %w", err)
	}

	fmt.Printf("Updating %s\n", site.Name)

	passwordless := noPassword || site.Config.PasswordlessSudo
	needsPassword := false
	if !passwordless {
		needsPassword = site.Config.ConfigPath != ""
		for _, cmd := range updateCmds {
			if strings.Contains(cmd, placeholderSudoPW) || strings.Contains(cmd, placeholderSudoPWB64) {
				needsPassword = true
			}
		}
	}

	var password, passwordB64 string
	if needsPassword {
		var err error
		password, err = r.promptPassword("Enter sudo password: ")
		if err != nil {
			return err
		}
		passwordB64 = encodePasswordBase64(password)
	}

	if site.Config.ConfigPath != "" {
		if err := pushConfig(r, site, passwordB64, passwordless); err != nil {
			return err
		}
	}

	for i, raw := range updateCmds {
		var actual, loggable string
		if passwordless {
			actual = rewriteForPasswordlessSudo(raw)
			loggable = actual
		} else {
			actual, loggable = substituteSudoPassword(raw, password, passwordB64)
		}
		sshCmd := buildSSHCommand(site.Name, actual)
		sshLog := buildSSHCommand(site.Name, loggable)
		exitCode := r.run(sshCmd, sshLog, os.Stdout, os.Stderr, true)

		// AIDE returns 0-7 as informational bitflags for the check/update
		// command; 14-23 are real errors. The first command in the sequence
		// is treated as the AIDE invocation. Subsequent commands (e.g. mv)
		// must return 0.
		if i == 0 {
			if exitCode > aideStatusMax || exitCode < 0 {
				return fmt.Errorf("AIDE command %s", formatExitError(exitCode))
			}
		} else if exitCode != 0 {
			return fmt.Errorf("command %s", formatExitError(exitCode))
		}
	}
	return nil
}

// substituteSudoPassword returns (actual, loggable) versions of cmd.
// `actual` has placeholders replaced with the real password and may have
// `sudo` rewritten to `sudo -S` to consume the piped password.
// `loggable` is the same shape but keeps {SUDO_PW_B64} as a placeholder so
// it can be safely written to the log file.
func substituteSudoPassword(cmd, password, passwordB64 string) (actual, loggable string) {
	if password == "" {
		return cmd, cmd
	}
	actual = strings.ReplaceAll(cmd, placeholderSudoPWB64, passwordB64)
	if strings.Contains(actual, placeholderSudoPW) {
		if containsShellSpecialChars(password) {
			fmt.Fprintf(os.Stderr, "Warning: password contains special characters. Consider using %s with base64 decoding.\n", placeholderSudoPWB64)
		}
		actual = strings.ReplaceAll(actual, placeholderSudoPW, escapeForSingleQuotes(password))
	}
	actual = injectSudoPassword(actual, passwordB64)
	loggable = injectSudoPassword(cmd, placeholderSudoPWB64)
	return actual, loggable
}

// injectSudoPassword rewrites a remote command that still uses plain `sudo`
// (i.e. not `sudo -S` or `sudo -n`) so the password is supplied via stdin.
// ssh runs without a TTY, so otherwise sudo can't prompt and the command
// fails with "a terminal is required to read the password". Only the first
// `sudo ` is rewritten; subsequent invocations rely on sudo's timestamp
// caching from the first one.
func injectSudoPassword(cmd, pwB64 string) string {
	if pwB64 == "" || !strings.Contains(cmd, "sudo ") {
		return cmd
	}
	if strings.Contains(cmd, "sudo -S") || strings.Contains(cmd, "sudo -n") {
		return cmd
	}
	return fmt.Sprintf("echo %s|base64 -d|%s", pwB64, strings.Replace(cmd, "sudo ", "sudo -S ", 1))
}

func encodePasswordBase64(password string) string {
	return base64.StdEncoding.EncodeToString([]byte(password))
}

// rewriteForPasswordlessSudo prepares a command for hosts where the SSH
// user has passwordless sudo. The {SUDO_PW*} placeholders are stripped
// (any leftover `echo |base64 -d|...` pipeline harmlessly pipes empty
// stdin into the next command, which ignores it). `sudo -S` is switched
// to `sudo -n`, and bare `sudo` is also given `-n` so it fails fast
// instead of trying to open a TTY ssh hasn't allocated.
func rewriteForPasswordlessSudo(cmd string) string {
	cmd = strings.ReplaceAll(cmd, placeholderSudoPWB64, "")
	cmd = strings.ReplaceAll(cmd, placeholderSudoPW, "")
	cmd = strings.Replace(cmd, "sudo -S ", "sudo -n ", 1)
	if !strings.Contains(cmd, "sudo -n") && strings.Contains(cmd, "sudo ") {
		cmd = strings.Replace(cmd, "sudo ", "sudo -n ", 1)
	}
	return cmd
}

// pushConfig copies the local db/<site>/aide.conf (file mode) or
// db/<site>/aide/ tree (dir mode) onto the remote at site.Config.ConfigPath,
// using sudo so it can land in root-owned locations like /etc/aide/.
// When passwordless is true, the remote sudo invocation uses -n instead
// of piping a base64 password via -S.
func pushConfig(r *Runner, site Site, pwB64 string, passwordless bool) error {
	remotePath := site.Config.ConfigPath
	localDir := filepath.Join(site.Dir, "aide")
	localFile := filepath.Join(site.Dir, "aide.conf")

	dirInfo, dirErr := os.Stat(localDir)
	fileInfo, fileErr := os.Stat(localFile)

	tmp := fmt.Sprintf(remoteTmpTemplate, site.Name)

	prep := fmt.Sprintf("rm -rf %s && mkdir -p %s", tmp, tmp)
	if code := r.run(buildSSHCommand(site.Name, prep), buildSSHCommand(site.Name, prep), os.Stdout, os.Stderr, true); code != 0 {
		return fmt.Errorf("prepare remote temp dir %s", formatExitError(code))
	}

	var sudoCmds []string
	switch {
	case dirErr == nil && dirInfo.IsDir():
		fmt.Printf("Pushing %s/ -> %s:%s\n", localDir, site.Name, remotePath)
		if err := r.scp(localDir+"/.", site.Name+":"+tmp, true); err != nil {
			return err
		}
		sudoCmds = []string{
			fmt.Sprintf("sudo /bin/mkdir -p %s", remotePath),
			fmt.Sprintf("sudo /bin/cp -r %s/. %s", tmp, remotePath),
		}
	case fileErr == nil && !fileInfo.IsDir():
		fmt.Printf("Pushing %s -> %s:%s\n", localFile, site.Name, remotePath)
		if err := r.scp(localFile, site.Name+":"+tmp+"/aide.conf", false); err != nil {
			return err
		}
		sudoCmds = []string{
			fmt.Sprintf("sudo /bin/cp %s/aide.conf %s", tmp, remotePath),
		}
	default:
		return fmt.Errorf("config_path is set but neither %s nor %s exists locally — run --fetch first", localFile, localDir)
	}

	// Rewrite each sudo invocation for the auth mode in use. In password
	// mode, only the first command pipes the password via -S; sudo's
	// timestamp cache covers any follow-ups, which use -n to fail fast if
	// the cache is unexpectedly cold.
	actualCmds := make([]string, len(sudoCmds))
	logCmds := make([]string, len(sudoCmds))
	for i, cmd := range sudoCmds {
		switch {
		case passwordless:
			rewritten := strings.Replace(cmd, "sudo ", "sudo -n ", 1)
			actualCmds[i] = rewritten
			logCmds[i] = rewritten
		case i == 0:
			withS := strings.Replace(cmd, "sudo ", "sudo -S ", 1)
			actualCmds[i] = fmt.Sprintf("echo %s|base64 -d|%s", pwB64, withS)
			logCmds[i] = fmt.Sprintf("echo %s|base64 -d|%s", placeholderSudoPWB64, withS)
		default:
			rewritten := strings.Replace(cmd, "sudo ", "sudo -n ", 1)
			actualCmds[i] = rewritten
			logCmds[i] = rewritten
		}
	}

	// Chain with && so a failure short-circuits; always clean up tmp and
	// propagate the chain's exit code so aided sees the real failure.
	chain := strings.Join(actualCmds, " && ")
	logChain := strings.Join(logCmds, " && ")
	actualRemote := fmt.Sprintf("%s; ec=$?; rm -rf %s; exit $ec", chain, tmp)
	logRemote := fmt.Sprintf("%s; ec=$?; rm -rf %s; exit $ec", logChain, tmp)

	if code := r.run(buildSSHCommand(site.Name, actualRemote), buildSSHCommand(site.Name, logRemote), os.Stdout, os.Stderr, true); code != 0 {
		return fmt.Errorf("remote sudo cp %s", formatExitError(code))
	}
	return nil
}
