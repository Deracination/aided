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

This is a single-file Go application (~524 lines in main.go) that automates AIDE integrity checks across remote systems via SSH. The tool runs checks in parallel, handles SSH retries, and supports email notifications.

## Architecture

**Single-file design**: All logic is in `main.go` with no package separation.

**Key components in main.go**:
- `main()` - CLI parsing, signal handling, site discovery, orchestration
- `getSites()` - Discovers sites from db directory
- `checkSite()` - Runs AIDE check on a site with SSH retry logic
- `updateSite()` - Updates AIDE database with password handling
- `runRemote()` - SSH command execution with context cancellation
- `runParallelChecks()` - Semaphore-based concurrency control

**Configuration hierarchy**: Sites are directories under `db/`. Each site can have a `config.json` that overrides default AIDE commands.

**Concurrency model**: Uses goroutines with WaitGroup synchronization and semaphore pattern for limiting parallel checks. Context-based cancellation enables graceful shutdown on Ctrl+C.

## Site Configuration

Sites are directories in `db/{site}/` with optional `config.json`:

```json
{
    "check": "sudo /usr/bin/aide --config=/etc/aide/aide.conf --check",
    "update": "sudo /usr/sbin/aideinit -y -f"
}
```

The `update` field can be a string or an array of commands.
