# AIDE Checker
Run system integrity checks using [AIDE](https://aide.github.io/) against remote systems.

The AIDE executable, database, and configuration are stored on the local system (where checker is run)
and copied to the remote system where AIDE is run.

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
## Requires
*   Perl v5.20+ on checker system
*   AIDE installed on remote systems

