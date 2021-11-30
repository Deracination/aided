# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems via ssh, scp, and sudo.

The account running *checker* must be able to ssh to each site and run sudo on the remote system.

The AIDE executable, database, and configuration are stored on the local system (where checker is run)
and copied to the remote system where AIDE is run (via ssh sudo).

Configuration files are expected to live in a db directory in the same directory as the checker program, unless
overridden by the --db command-line parameter.

## Usage
Check (runs AIDE check) all sites listed in the ~/db directory

    checker

Checks all sites listed in the ~/db directory and sends email to *person@somewhere.com* if any checks fail.

    checker --mail=person@somewhere.com

Checks just the *site1* site which must be present in the ~/db directory

    checker --check site1

Updates the stored database for *new_site*. Use this to update aide if you make some file changes on *new_site* (like installing updates with yum, etc). If you don't have an existing configuration for *new_site* it will be created and added to ~/db.

    checker --update new_site

Checks all sites listed in the /opt/checker/db directory

    checker --db=/opt/checker/db
## Options
* --mail=address : if set the result of running a check is emailed to this address.
* --who=username : set the username who is ssh'd into the remote systems (defaults to user running checker). The user will be set as owner of copies files which will be scp'd back to the local system (aide.db.gz, aide executable). The user is only required for --update and --fetch. Not required for running checks of systems.

## Requires
*   Perl v5.20+ on checker system
*   AIDE installed on remote systems

