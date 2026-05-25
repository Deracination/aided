package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	exitCodeCancelled = -2
	exitCodeSpawnErr  = -1
)

// Runner owns the mutable per-process state (context, log file, terminal
// state for password prompts) that was previously held in package globals.
// One Runner is constructed in main and threaded through every function
// that runs SSH commands or coordinates with the signal handler.
type Runner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	logFile *os.File

	ttyState    atomic.Pointer[term.State]
	logWarnOnce sync.Once
	signalsSeen atomic.Int32
}

func newRunner(logPath string) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{ctx: ctx, cancel: cancel}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open log file %s: %v\n", logPath, err)
	} else {
		r.logFile = f
	}
	return r
}

func (r *Runner) installSignalHandler() {
	sigChan := make(chan os.Signal, 4)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range sigChan {
			n := r.signalsSeen.Add(1)
			r.restoreTTY()
			if n == 1 {
				printq("\nReceived %s, cancelling...\n", sig)
				r.cancel()
			} else {
				fmt.Fprintln(os.Stderr, "\nReceived second signal, force-exiting.")
				os.Exit(130)
			}
		}
	}()
}

func (r *Runner) restoreTTY() {
	if state := r.ttyState.Load(); state != nil {
		_ = term.Restore(int(syscall.Stdin), state)
	}
}

func (r *Runner) logf(format string, a ...any) {
	if r.logFile == nil {
		return
	}
	line := fmt.Sprintf(format, a...)
	if _, err := fmt.Fprintf(r.logFile, "[%s] %s\n", time.Now().Format(time.RFC3339), line); err != nil {
		r.warnLogFailure(err)
	}
}

func (r *Runner) warnLogFailure(err error) {
	r.logWarnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "Warning: log file write failed: %v (further log failures will be silent)\n", err)
	})
}

// run executes cmdLine via /bin/sh -c. logLine is the form written to the log
// file and echoed under -v; pass a redacted copy if cmdLine contains secrets.
// An empty logLine suppresses both logging and the verbose echo — used for
// internal probes (e.g. test -d on the remote) that shouldn't appear in the
// user-visible log.
//
// If useCtx is true, the command is killed when the Runner's context is
// cancelled (Ctrl+C). nil writers default to io.Discard.
func (r *Runner) run(cmdLine, logLine string, stdout, stderr io.Writer, useCtx bool) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	if logLine != "" {
		r.logf("%s", logLine)
		if verbose {
			fmt.Fprintln(stdout, logLine)
		}
	}

	parentCtx := context.Background()
	if useCtx {
		parentCtx = r.ctx
	}
	cmd := exec.CommandContext(parentCtx, "/bin/sh", "-c", cmdLine)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var logBuf bytes.Buffer
	captureForLog := logLine != "" && r.logFile != nil
	if captureForLog {
		cmd.Stdout = io.MultiWriter(stdout, &logBuf)
		cmd.Stderr = io.MultiWriter(stderr, &logBuf)
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if useCtx && r.ctx.Err() != nil {
			exitCode = exitCodeCancelled
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = exitCodeSpawnErr
		}
	}

	if captureForLog && logBuf.Len() > 0 {
		if _, werr := r.logFile.Write(logBuf.Bytes()); werr != nil {
			r.warnLogFailure(werr)
		}
	}
	if logLine != "" && exitCode != 0 {
		r.logf("Exit code: %d", exitCode)
	}

	return exitCode
}

// probe runs a remote command and returns trimmed stdout and a success flag.
// Internal probes are not echoed or logged.
func (r *Runner) probe(host, remoteCmd string) (string, bool) {
	var buf bytes.Buffer
	sshCmd := buildSSHCommand(host, remoteCmd)
	code := r.run(sshCmd, "", &buf, io.Discard, false)
	return strings.TrimSpace(buf.String()), code == 0
}

// promptPassword reads a password from the terminal, saving the terminal
// state so a SIGINT during entry can restore it before exit.
func (r *Runner) promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	if state, err := term.GetState(int(syscall.Stdin)); err == nil {
		r.ttyState.Store(state)
		defer r.ttyState.Store(nil)
	}
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Println()
	return string(bytePassword), nil
}
