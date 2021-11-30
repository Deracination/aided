# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems via ssh, scp, and sudo.

## Why This Exists
It is perfectly possible to install AIDE and run it on systems to ensure no changes have been made to files. However, an
intruder discovering a cron job running AIDE may be able to defeat the system checks (by updating the AIDE db after breaking in).

It's a little harder to defeat AIDE when the executable and the comparison database are copied onto the system before running. This
script provides a reasonably simple way to automate this process for any number of servers.

## Details

The *checker* script by default copies the aide db and executable to each server listed in the db directory, runs AIDE
and reports any detected problems.

This requires an ssh login to each server which can run sudo (via ssh).

The AIDE executable, database, and configuration are stored on the local system (where checker is run)
and copied to the remote server where AIDE is run (via ssh sudo).

## Usage
Check (runs AIDE check) all servers listed in the ~/db directory

    checker

Checks all servers listed in the ~/db directory and sends email to *person@somewhere.com* if any checks fail.

    checker --mail=person@somewhere.com

Checks just the *site1* servers which must be present in the ~/db directory

    checker --check site1

Updates the stored database for *new_site*. Use this to update aide if you make some file changes on *new_site* (like installing updates with yum, etc). If you don't have an existing configuration for *new_site* it will be created and added to ~/db.

    checker --update new_site

Checks all sites listed in the /opt/checker/db directory

    checker --db=/opt/checker/db
## Options
* --mail=address : if set the result of running a check is emailed to this address.
* --who=username : set the username who is ssh'd into the remote systems (defaults to user running checker). The user will be set as owner of copies files which will be scp'd back to the local system (aide.db.gz, aide executable). The user is only required for --update and --fetch. Not required for running checks of systems.

## Requires
*   Perl v5.20+ on checker system (locally)
*   AIDE installed on remote systems
*   SSH login from local system which can run sudo on the remote systems.

