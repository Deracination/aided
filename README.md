# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems via ssh, scp, and sudo.

## Why This Exists
It is perfectly possible to install AIDE and run it on systems to ensure no changes have been made to files. However, an
intruder discovering a cron job running AIDE may be able to defeat the system checks (by updating the AIDE db after breaking in).

It's a little harder to defeat AIDE when the executable and the comparison database are copied onto the system before running. This
script provides a reasonably simple way to automate this process for any number of servers.

## Implementations

This project provides two implementations:

1. **`checker`** - Original Perl implementation (requires Perl v5.20+)
2. **`checker.sh`** - Bash/Ansible reimplementation (requires Bash 4+ and optionally Ansible)

Both implementations share the same configuration files in the `db/` directory.

---

## Bash/Ansible Implementation (`checker.sh`)

### Usage

Check all servers listed in the `db/` directory:
```bash
./checker.sh
```

Check specific sites:
```bash
./checker.sh site1 site2
```

Check all servers and email results:
```bash
./checker.sh --mail=person@somewhere.com
```

Update the AIDE database for a site:
```bash
./checker.sh --update=mysite
```

Use a custom database directory:
```bash
./checker.sh --db=/opt/checker/db
```

Enable verbose output:
```bash
./checker.sh --verbose
```

### Options

| Option | Description |
|--------|-------------|
| `--db=DIR` | Override database directory (default: `./db`) |
| `--update=NAME` | Update AIDE database for specified site |
| `--mail=ADDRESS` | Email results to this address |
| `--who=USERNAME` | SSH username (default: current user) |
| `--verbose` | Enable verbose output |
| `-h, --help` | Show help message |

### Requirements

- Bash 4.0+
- `jq` (for JSON parsing)
- SSH access to remote systems
- `sudo` privileges on remote systems
- AIDE installed on remote systems
- (Optional) Ansible 2.9+ for playbook-based checks

### Using Ansible Playbooks Directly

You can also use the Ansible playbooks directly for more control:

**Run checks on specific hosts:**
```bash
ansible-playbook -i inventory.ini playbooks/check.yml
```

**Update AIDE database:**
```bash
ansible-playbook -i inventory.ini playbooks/update.yml \
    -e "target_host=mysite" \
    -e "sudo_password=secret"
```

**Example inventory file (`inventory.ini`):**
```ini
[aide_hosts]
webserver1 ansible_host=192.168.1.10 ansible_user=admin
webserver2 ansible_host=192.168.1.11 ansible_user=admin
dbserver ansible_host=192.168.1.20 ansible_user=dbadmin
```

---

## Original Perl Implementation (`checker`)

### Usage

Check all servers listed in the `db/` directory:
```bash
./checker
```

Check all servers and send email if any checks fail:
```bash
./checker --mail=person@somewhere.com
```

Check specific sites:
```bash
./checker site1 site2
```

Update the stored database for a site:
```bash
./checker --update=new_site --who=username
```

Use a custom database directory:
```bash
./checker --db=/opt/checker/db
```

### Options

| Option | Description |
|--------|-------------|
| `--db=DIR` | Override database directory |
| `--update=NAME` | Update AIDE database for specified site |
| `--mail=ADDRESS` | Email check results to this address |
| `--who=USERNAME` | SSH username for remote systems |
| `--verbose` | Enable verbose SSH/SCP output |

### Requirements

- Perl v5.20+ on the local system
- AIDE installed on remote systems
- SSH login from local system with sudo privileges on remote systems

---

## Site Configuration

Site configurations are stored in `db/<sitename>/config.json`. This allows customization per site to support different Linux distributions (Ubuntu, AlmaLinux, etc.) which may have AIDE installed in different locations.

### Example Configuration

```json
{
    "check": "sudo /usr/sbin/aide --check",
    "update": [
        "sudo /usr/sbin/aide --update",
        "echo {SUDO_PW}|sudo -S mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz"
    ],
    "host": "192.168.1.10",
    "user": "admin"
}
```

### Configuration Options

| Option | Description |
|--------|-------------|
| `check` | Command to run AIDE check (default: `sudo /usr/sbin/aide --check`) |
| `update` | Command(s) to update AIDE database (can be string or array) |
| `host` | Custom hostname/IP for SSH connection |
| `user` | Custom SSH username for this site |

### Default Paths

- AIDE executable: `/usr/sbin/aide`
- AIDE home/database: `/var/lib/aide`
- AIDE database file: `aide.db.gz`

---

## Directory Structure

```
aided/
├── checker           # Original Perl implementation
├── checker.sh        # Bash/Ansible reimplementation
├── ansible.cfg       # Ansible configuration
├── playbooks/
│   ├── check.yml     # AIDE check playbook
│   └── update.yml    # AIDE update playbook
├── db/               # Site configurations and data
│   ├── site1/
│   │   └── config.json
│   ├── site2/
│   │   └── config.json
│   └── ...
└── README.md
```

---

## How It Works

1. The checker reads site configurations from the `db/` directory
2. For each site, it establishes an SSH connection
3. AIDE is executed via `sudo` to check file integrity
4. Results are collected and reported
5. Optionally, results are emailed to administrators

### Security Model

- AIDE database and configuration are stored on the checking system (not on monitored servers)
- SSH connections use strict host key checking disabled for automation
- Sudo passwords can be provided interactively for update operations
- Retry logic handles transient network failures

---

## Cron Example

Run daily AIDE checks and email results:

```cron
0 3 * * * /path/to/aided/checker.sh --mail=security@example.com 2>&1 | logger -t aide-checker
```

Or using the Perl version:

```cron
0 3 * * * /path/to/aided/checker --mail=security@example.com 2>&1 | logger -t aide-checker
```
