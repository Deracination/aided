# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems via ssh, scp, and sudo.

## Why This Exists
It is perfectly possible to install AIDE and run it on systems to ensure no changes have been made to files. However, an
intruder discovering a cron job running AIDE may be able to defeat the system checks (by updating the AIDE db after breaking in).

It's a little harder to defeat AIDE when the executable and the comparison database are copied onto the system before running. This
script provides a reasonably simple way to automate this process for any number of servers.

## Details

The *aided* tool runs AIDE checks on each server listed in the db directory and reports any detected problems.

This requires an ssh login to each server which can run sudo (via ssh).

Site-specific configurations can be stored in JSON files at `db/{site}/config.json`.

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
## Options
* --mail=address : if set the result of running a check is emailed to this address.
* --who=username : set the username who is ssh'd into the remote systems (defaults to user running checker). The user will be set as owner of copies files which will be scp'd back to the local system (aide.db.gz, aide executable). The user is only required for --update and --fetch. Not required for running checks of systems.

## Requires
*   Go 1.21+ to build from source (or use pre-built binary)
*   AIDE installed on remote systems
*   SSH login from local system which can run sudo on the remote systems.

## Building from Source

```bash
go build -o aided .
```

