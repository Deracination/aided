# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems via ssh, scp, and sudo.

## Why This Exists
It is perfectly possible to install AIDE and run it on systems to ensure no changes have been made to files. However, an
intruder discovering a cron job running AIDE may be able to defeat the system checks (by updating the AIDE db after breaking in).

It's a little harder to defeat AIDE when the comparison database is copied onto the system before running. This
script provides a reasonably simple way to automate this process for any number of servers.

## Details

The *aided* tool runs AIDE checks on each server listed in the db directory and reports any detected problems.

This requires an ssh login to each server which can run sudo.

Site-specific configurations can be stored in JSON files at `db/{site}/config.json`.

## Setting Up a Remote Server

Each server that `aided` checks needs an SSH login, a small set of standard utilities, and a sudoers drop-in that lets the login user run AIDE without a password. The check flow runs without a TTY and does **not** pipe a sudo password, so passwordless sudo on the AIDE commands is effectively required.

### SSH access

- The tool shells out to `/usr/bin/ssh` and `/usr/bin/scp`. The **site name** (the directory name under `db/`) is passed verbatim as the SSH host argument — so `db/web01/` produces `ssh web01 …`. Use a `Host` block in `~/.ssh/config` to map that alias to the real hostname, user, and key.
- Connections use these hard-coded options: `-q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null`. Host-key prompts are suppressed; don't rely on `known_hosts` pinning for these connections.
- **Key-based auth only** — the tool will not type an SSH password. Add the local user's public key to the remote user's `~/.ssh/authorized_keys`.

Example `~/.ssh/config` snippet:

```
Host web01
    HostName web01.example.com
    User aided
    IdentityFile ~/.ssh/id_ed25519_aided
```

### Software required on the remote

- `aide` binary — defaults assume `/usr/sbin/aide`. `aided --init <site>` probes `/usr/sbin/aide`, `/usr/bin/aide`, and `/usr/local/bin/aide`, and records whichever it finds in `db/<site>/config.json`. If yours lives elsewhere, pass `--aide=<path>`.
- `aideinit` (optional) — if present, `--init` writes a one-line `aideinit`-style update command instead of the two-step `aide --update` + `mv` pair.
- `sudo`, `sh`, `mkdir`, `cp`, `rm`, `mv`, `base64`, `test` — all standard POSIX, pre-installed on every supported distro.
- `/tmp` must be writable by the SSH user (the tool stages config pushes under `/tmp/aided-<site>/`).
- `/var/lib/aide/` must be writable by the AIDE process (root). The SSH user does not need direct access to it.

### Sudoers drop-in

Install at `/etc/sudoers.d/aided` on the remote, mode `0440`, owner `root:root`. Validate with `visudo -cf /etc/sudoers.d/aided` before placing it in `/etc/sudoers.d/`.

```
# /etc/sudoers.d/aided — passwordless sudo for the aided integrity-check tool.
# Replace USERNAME with the SSH user (e.g. "aided").
# Replace /etc/aide/aide.conf with your actual config_path if it differs.

# Some older distros default to requiretty; sshd-driven sessions have no tty.
Defaults:USERNAME !requiretty

# --- Check flow (required) ---
USERNAME ALL=(root) NOPASSWD: /usr/sbin/aide --check
USERNAME ALL=(root) NOPASSWD: /usr/sbin/aide --config=/etc/aide/aide.conf --check

# --- Update flow (if you use --update=<site>) ---
USERNAME ALL=(root) NOPASSWD: /usr/sbin/aide --update
USERNAME ALL=(root) NOPASSWD: /usr/sbin/aide --config=/etc/aide/aide.conf --update
USERNAME ALL=(root) NOPASSWD: /bin/mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz

# OR: --- Update flow (if you use aideinit-style update ---
USERNAME ALL=(root) NOPASSWD: /usr/sbin/aideinit -y -f -- --config=/etc/aide/aide.conf

# --- Config push (required if config_path is set in db/<site>/config.json) ---
# Directory-mode config_path (e.g. /etc/aide) — needs both:
USERNAME ALL=(root) NOPASSWD: /bin/mkdir -p /etc/aide
USERNAME ALL=(root) NOPASSWD: /bin/cp -r /tmp/aided-*/. /etc/aide
# File-mode config_path (e.g. /etc/aide/aide.conf) — needs:
USERNAME ALL=(root) NOPASSWD: /bin/cp /tmp/aided-*/aide.conf /etc/aide/aide.conf
```

Notes:

- `/bin/mv`, `/bin/mkdir`, and `/bin/cp` may live under `/usr/bin/` on systemd-merged distros (Fedora 17+, modern Debian/Ubuntu). Check `which mv mkdir cp` and adjust the paths.
- The config-push rules hard-code the **target** path (`/etc/aide` or `/etc/aide/aide.conf`). If your `config_path` differs, edit the literal accordingly — do **not** replace it with `*`, since that would grant write access anywhere.
- The check/update commands above match the defaults baked into `aided`. If you override `check`/`update` in `db/<site>/config.json`, add matching sudoers lines for whatever commands you put there.
- Once NOPASSWD sudo is in place, tell `aided` to skip the password prompt on `--update`: either set `"passwordless_sudo": true` in `db/<site>/config.json`, or pass `--nopassword` on the command line. With either set, sudo invocations are rewritten to use `-n` so they fail fast if NOPASSWD isn't configured correctly.

### Step-by-step setup

1. On the remote host, create the service user and install its key:

   ```
   sudo useradd -m -s /bin/bash aided
   sudo -u aided mkdir -m 700 /home/aided/.ssh
   sudo -u aided tee /home/aided/.ssh/authorized_keys < your-pubkey.pub
   sudo chmod 600 /home/aided/.ssh/authorized_keys
   ```

2. Install AIDE (`apt install aide aide-common` on Debian/Ubuntu, `dnf install aide` on RHEL/Fedora) and initialize its database (`sudo aideinit`, or `sudo aide --init && sudo mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz`).
3. Install the sudoers drop-in above as `/etc/sudoers.d/aided` (with `USERNAME` substituted), `chmod 0440`, validate with `visudo -cf /etc/sudoers.d/aided`.
4. On your workstation, add a `Host` entry to `~/.ssh/config` pointing the site alias at the remote.
5. Run `aided --init <site>` from the repo to write `db/<site>/config.json`. This probes the remote for `aide`, `aideinit`, and `aide.conf` and picks paths accordingly.
6. Smoke-test: `aided <site>` for a check, `aided --update=<site>` for an update.

## Usage
Check (runs AIDE check) all servers listed in the ./db directory

    ./aided

Checks all servers listed in the ./db directory and sends email to *person@somewhere.com* if any checks fail.

    ./aided --mail=person@somewhere.com

Checks just the *site1* server which must be present in the ./db directory

    ./aided site1

Updates the stored database for *new_site*. Use this to update aide if you make some file changes on *new_site* (like installing updates with yum, etc). If you don't have an existing configuration for *new_site* it will be created and added to ./db.

    ./aided --update=new_site

Checks all sites listed in the /opt/checker/db directory

    ./aided --db=/opt/checker/db

Fetches the remote `aide.conf` for *site1* into `db/site1/` (file mode) or `db/site1/aide/` (when the remote path is a directory), and records the path as `config_path` in `db/site1/config.json`:

    ./aided --fetch=/etc/aide/aide.conf site1
    ./aided --fetch=/etc/aide site1

Once `config_path` is recorded you can re-pull without repeating the path:

    ./aided --fetch site1

When `config_path` is set, `--update=site1` first pushes the local config back to the remote (via `scp` to `/tmp/aided-site1/` + `sudo cp`) so AIDE rebuilds its database against the version-controlled config.

## Options
* --mail=address : if set the result of running a check is emailed to this address.
* --who=username : set the username who is ssh'd into the remote systems (defaults to user running checker). The user will be set as owner of copies files which will be scp'd back to the local system (aide.db.gz, aide executable). The user is only required for --update and --fetch. Not required for running checks of systems.
* --fetch[=path] site : pull the remote aide config into `db/site/`. With `=path`, the path is also written to `config.json`. Without `=path`, the path stored in `config.json` is reused; if neither is available the command exits with an error.
* --nopassword : skip the sudo password prompt on `--update`; sudo invocations are rewritten to `sudo -n`. The same effect can be made persistent per-site with `"passwordless_sudo": true` in `db/<site>/config.json`.

## Requires
*   Go 1.21+ to build from source
*   SSH + sudo access on each remote — see [Setting Up a Remote Server](#setting-up-a-remote-server) above for the exact requirements and a ready-to-use sudoers drop-in.

## Building from Source

### Prerequisites

- Go 1.21 or later installed

### Build Steps

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd <repository-directory>
   ```

2. Download dependencies:
   ```bash
   go mod download
   ```

3. Build the binary:
   ```bash
   go build -o aided .
   ```

4. (Optional) Install to your PATH:
   ```bash
   go install .
   ```

### Verify the Build

```bash
./aided --help
```

