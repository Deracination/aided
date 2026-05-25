# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Download dependencies
go mod download

# Build the binary
go build -o aided .

# Install to PATH
go install .

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o aided-linux-amd64 .
```

## Project Overview

This is a Go-lang application that automates AIDE integrity checks across remote systems via SSH. The tool runs checks in parallel, handles SSH retries, and supports email notifications.

## Architecture

The code is split across files in `package main`, organised by concern:

| File | Contents |
|------|----------|
| `main.go` | `main()`, flag parsing (`parseFlags`), `printUsage`, `sendMail`, the `verbose`/`sshVerbose`/`quiet` globals, and the `printq`/`fprintq` output helpers. |
| `runner.go` | The `Runner` type — owns ctx/cancel/log file/tty state — plus `newRunner`, the signal handler, the unified SSH runner `Runner.run`, `Runner.probe`, and `Runner.promptPassword`. |
| `ssh.go` | `buildSSHCommand`, `shellQuote`, `escapeForSingleQuotes`, `containsShellSpecialChars`, `Runner.scp`, `Runner.remoteIsDir`, plus the `sshOpts` global and SSH retry constants. |
| `site.go` | `Site`, `Config`, `loadSite`, `getSites`, `getConfig`, `getUpdateCommands`, `checkSite`, `runParallelChecks`, `runChecks`, `filterSites`, `getMessage`, `formatExitError`, and AIDE exit-code constants. |
| `update.go` | `runUpdate`, `pushConfig`, `substituteSudoPassword`, `injectSudoPassword`, `encodePasswordBase64`, `rewriteForPasswordlessSudo`, and the `{SUDO_PW*}` placeholder constants. |
| `fetch.go` | `runFetch`, `readConfigMap`, `writeConfigMap`, `rewriteBareFetch`, and `fetchUnset`. |
| `init.go` | `runInit`, `Runner.findRemoteBinary`, `Runner.findRemoteFile`, `Runner.findRemotePath`, `promptPath`. |
| `config.go` | `loadAidedConf`, `readAidedConf` — load `key=value` defaults from `/etc/aided.conf` and `~/.aided.conf`, applied via `flag.Set` before `flag.Parse`. |

`main()` is a thin dispatch shell: parse flags, construct a `Runner`, install the signal handler, then call one of `runInit` / `runFetch` / `runUpdate` / `runChecks`.

**Configuration hierarchy**: Sites are directories under `db/`. Each site can have a `config.json` that overrides default AIDE commands.

**Concurrency model**: Uses goroutines with WaitGroup synchronization and a semaphore channel for limiting parallel checks. The `Runner` carries a `context.Context`; Ctrl+C cancels it, which kills any active SSH child via `cmd.Cancel` (SIGKILL to the process group). A second signal force-exits with code 130.

**Logging**: `Runner.run` takes a separate `logLine` parameter so commands containing sudo passwords can be logged with the password as a placeholder while the executed command carries the real value. The `aided.log` file never sees secrets.

## Site Configuration

Sites are directories in `db/{site}/` with optional `config.json`:

```json
{
    "check": "sudo /usr/bin/aide --config=/etc/aide/aide.conf --check",
    "update": "sudo /usr/sbin/aideinit -y -f",
    "config_path": "/etc/aide/aide.conf"
}
```

The `update` field can be a string or an array of commands.

`config_path` points at the remote `aide.conf` (or the directory holding `aide.conf` plus its `@@include` files). When set:

- `aided --fetch[=<path>] <site>` pulls the remote config into `db/{site}/aide.conf` (file mode) or `db/{site}/aide/` (directory mode), persisting `<path>` to `config.json` when provided.
- `aided --update=<site>` pushes that local copy back up (`scp` to `/tmp/aided-<site>/` then `sudo cp` into `config_path`) before running the AIDE update, so the rebuilt database reflects the version-controlled config.

`passwordless_sudo: true` (or the `--nopassword` CLI flag) marks a site as having NOPASSWD sudo for its AIDE commands. `runUpdate` then skips the password prompt and rewrites sudo calls (including the hard-coded `sudo -S` in `pushConfig` and the default `{SUDO_PW_B64}|base64 -d|sudo -S mv` db-move) to use `sudo -n` instead.
