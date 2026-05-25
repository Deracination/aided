package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	verbose    bool
	sshVerbose bool
	quiet      bool
)

// printq prints to stdout unless quiet mode is enabled.
func printq(format string, a ...any) {
	if !quiet {
		fmt.Printf(format, a...)
	}
}

// fprintq prints to a writer unless quiet mode is enabled.
func fprintq(w io.Writer, format string, a ...any) {
	if !quiet {
		fmt.Fprintf(w, format, a...)
	}
}

type cliArgs struct {
	dbDir          string
	update         string
	mail           string
	parallel       int
	initHost       string
	aidePath       string
	aideConfigPath string
	fetchPath      string
	fetchGiven     bool
	logPath        string
	noPassword     bool
	positional     []string
}

func printUsage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "Usage: %s [flags] [site ...]\n\nFlags:\n", filepath.Base(os.Args[0]))
	flag.PrintDefaults()

	fmt.Fprintln(out)
	if len(rcLoaded) > 0 {
		fmt.Fprintln(out, "Config files loaded (later overrides earlier):")
		for _, p := range rcLoaded {
			fmt.Fprintf(out, "    %s\n", p)
		}
	} else {
		fmt.Fprintln(out, "Config files: none found.")
	}
	if len(rcSearched) > 0 {
		fmt.Fprintf(out, "Searched: %s\n", strings.Join(rcSearched, ", "))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Effective values (rc-sourced entries are marked):")
	flag.VisitAll(func(f *flag.Flag) {
		cur := f.Value.String()
		if cur == "" {
			cur = `""`
		}
		suffix := ""
		if e, ok := rcSettings[f.Name]; ok {
			suffix = fmt.Sprintf("  [from %s]", e.source)
		}
		fmt.Fprintf(out, "    -%-11s %s%s\n", f.Name, cur, suffix)
	})

	fmt.Fprint(out, `
Global config file (/etc/aided.conf, ~/.aided.conf):

    Either file may set defaults for any long-form flag. Lines starting
    with # are comments; blank lines are ignored. Each non-comment line
    is key=value, where key is a long-form flag name. ~/.aided.conf
    overrides /etc/aided.conf; an explicit command-line flag overrides
    both. Example:

        # default database location for this user
        db=/var/lib/aided
        log=/var/log/aided.log
        p=4

Configuration file (db/<site>/config.json):

    {
        "check": "sudo /usr/sbin/aide --config=/etc/aide/aide.conf --check",
        "update": [
            "sudo /usr/sbin/aide --config=/etc/aide/aide.conf --update",
            "echo {SUDO_PW_B64}|base64 -d|sudo -S mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz"
        ],
        "config_path": "/etc/aide/aide.conf"
    }

Fields:
    check        Remote AIDE check command.
                 Default: "sudo /usr/sbin/aide --check".
    update       Remote update command(s). Either a string or an array of
                 strings run in sequence. Two placeholders are substituted
                 at runtime:
                     {SUDO_PW}      the sudo password, shell-escaped
                     {SUDO_PW_B64}  the sudo password, base64-encoded
                 Commands using plain "sudo " (no -S/-n) are automatically
                 rewritten to pipe the password via stdin.
    config_path  Optional. Path to the remote aide.conf, or to a directory
                 containing aide.conf plus its @@include files. When set,
                 --fetch pulls the remote config into db/<site>/, and
                 --update pushes the local copy back before rebuilding the
                 AIDE database.
    passwordless_sudo
                 Optional boolean. When true, --update skips the sudo
                 password prompt and rewrites sudo invocations to use -n.
                 The SSH user must have NOPASSWD sudo for the AIDE
                 binary (and any other commands in update). The same
                 effect can be applied per-invocation with --nopassword.
`)
}

func parseFlags() (*cliArgs, error) {
	rewriteBareFetch(&os.Args)

	args := &cliArgs{}

	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = os.Getwd()
	}
	defaultDbDir := filepath.Join(filepath.Dir(exePath), "db")
	if _, err := os.Stat(defaultDbDir); os.IsNotExist(err) {
		defaultDbDir = filepath.Join(".", "db")
	}

	flag.StringVar(&args.dbDir, "db", defaultDbDir, "database directory")
	flag.StringVar(&args.update, "u", "", "site to update")
	flag.StringVar(&args.mail, "m", "", "email address for notifications")
	flag.BoolVar(&verbose, "v", false, "print commands as they are executed")
	flag.BoolVar(&sshVerbose, "V", false, "enable SSH verbose output")
	flag.BoolVar(&quiet, "q", false, "quiet mode - suppress all output except errors")
	flag.IntVar(&args.parallel, "p", 0, "max parallel checks (0 = unlimited, 1 = sequential)")
	flag.StringVar(&args.logPath, "log", "./aided.log", "log file for SSH command output")
	flag.StringVar(&args.initHost, "init", "", "initialize configuration for a new host")
	flag.StringVar(&args.aidePath, "aide", "", "path to aide executable on remote host (used with --init)")
	flag.StringVar(&args.aideConfigPath, "config", "", "path to aide config file on remote host (used with --init)")
	flag.StringVar(&args.fetchPath, "fetch", fetchUnset, "fetch remote aide.conf into db/<site>/; takes optional =<path>")
	flag.BoolVar(&args.noPassword, "nopassword", false, "skip sudo password prompt; the SSH user must have passwordless sudo on the remote")
	flag.Usage = printUsage

	conf, err := loadAidedConf()
	if err != nil {
		return nil, err
	}
	for k, v := range conf {
		if err := flag.Set(k, v); err != nil {
			return nil, fmt.Errorf("aided.conf: %w", err)
		}
	}

	flag.Parse()

	if quiet {
		verbose = false
	}
	if sshVerbose {
		sshOpts = []string{"-v", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	}

	args.fetchGiven = args.fetchPath != fetchUnset
	if !args.fetchGiven {
		args.fetchPath = ""
	}
	args.positional = flag.Args()
	return args, nil
}

func sendMail(msg, address string) error {
	cmd := exec.Command("mail", "-s", "AIDE checks", address)
	cmd.Stdin = strings.NewReader(msg)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

func main() {
	args, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	r := newRunner(args.logPath)
	if r.logFile != nil {
		defer r.logFile.Close()
	}
	r.installSignalHandler()

	switch {
	case args.initHost != "":
		if err := runInit(r, args.initHost, args.aidePath, args.aideConfigPath, args.dbDir); err != nil {
			fmt.Fprintf(os.Stderr, "Init failed: %v\n", err)
			os.Exit(1)
		}
	case args.fetchGiven:
		if len(args.positional) == 0 {
			fmt.Fprintln(os.Stderr, "--fetch requires a site argument: aided --fetch[=<path>] <site>")
			os.Exit(1)
		}
		site, err := loadSite(args.positional[0], args.dbDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fetch failed: %v\n", err)
			os.Exit(1)
		}
		if err := runFetch(r, site, args.fetchPath); err != nil {
			fmt.Fprintf(os.Stderr, "Fetch failed: %v\n", err)
			os.Exit(1)
		}
	case args.update != "":
		site, err := loadSite(args.update, args.dbDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		if err := runUpdate(r, site, args.noPassword); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
	default:
		if err := runChecks(r, args); err != nil {
			if !errors.Is(err, errChecksFailed) {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(1)
		}
	}
}
