# AIDE Checker - C# .NET Port

This is a C# .NET port of the Perl AIDE checker tool. It performs AIDE (Advanced Intrusion Detection Environment) integrity checks on remote servers via SSH.

## Requirements

- .NET 8.0 SDK or later
- SSH access to remote servers (configured with key-based authentication)
- `mail` command (for email notifications, optional)

## Building

### Build for development/testing
```bash
dotnet build
```

### Build release version (framework-dependent)
```bash
dotnet publish -c Release
```
The executable will be in `bin/Release/net8.0/publish/checker`

### Build self-contained single file (recommended for deployment)
```bash
# For Linux x64
dotnet publish -c Release -r linux-x64 --self-contained -p:PublishSingleFile=true

# For Linux ARM64
dotnet publish -c Release -r linux-arm64 --self-contained -p:PublishSingleFile=true
```
The single-file executable will be in `bin/Release/net8.0/linux-x64/publish/checker`

## Installation

### Option 1: Framework-dependent (requires .NET runtime on target system)
```bash
dotnet publish -c Release
sudo cp bin/Release/net8.0/publish/checker /usr/local/bin/
sudo cp -r bin/Release/net8.0/publish/*.dll /usr/local/bin/
```

### Option 2: Self-contained (no .NET runtime required)
```bash
dotnet publish -c Release -r linux-x64 --self-contained -p:PublishSingleFile=true
sudo cp bin/Release/net8.0/linux-x64/publish/checker /usr/local/bin/
```

### Option 3: Run from build directory
```bash
dotnet run -- [options]
```

## Usage

The C# version maintains the same command-line interface as the Perl version:

### Check all sites
```bash
./checker
```

### Check specific sites
```bash
./checker site1 site2
```

### Update AIDE database for a site
```bash
./checker --update=sitename
```

### Specify custom database directory
```bash
./checker --db=/path/to/db
```

### Send email notifications
```bash
./checker --mail=admin@example.com
```

### Enable verbose output
```bash
./checker --verbose
```

### Combined example
```bash
./checker --db=/var/lib/aide-checker --mail=security@company.com --verbose site1 site2
```

## Configuration

Configuration works exactly like the Perl version:

1. Create a directory for each site in the `db/` directory
2. Optionally create `db/sitename/config.json` for site-specific settings

Example `config.json`:
```json
{
  "update": [
    "sudo /usr/bin/aide --update",
    "echo {SUDO_PW}|sudo -S mv /var/lib/aide/aide.db.new.gz /var/lib/aide/aide.db.gz"
  ],
  "check": "sudo /usr/bin/aide --check"
}
```

The `update` field can be either a string or an array of strings.

## SSH Configuration

The tool uses SSH.NET library which reads from standard SSH configuration (`~/.ssh/config`).

**Important**: Make sure you have:
1. SSH key-based authentication configured for all target hosts
2. Host entries in your `~/.ssh/config` or use fully qualified hostnames
3. Tested SSH connectivity manually before using the tool

Example `~/.ssh/config` entry:
```
Host myserver
    HostName myserver.example.com
    User admin
    IdentityFile ~/.ssh/id_rsa
    StrictHostKeyChecking no
```

## Differences from Perl Version

### Improvements
- **Type safety**: Compile-time checking prevents many runtime errors
- **Better async handling**: Uses async/await for cleaner concurrent operations
- **Structured command-line parsing**: Using System.CommandLine library
- **Native SSH library**: SSH.NET provides better error handling than shell commands
- **Single binary deployment**: Can be deployed as one self-contained executable

### Behavioral Changes
- SSH connection timeout is set to 30 seconds (configurable in code)
- Uses SSH.NET library instead of system `ssh` and `scp` commands
- Relies on SSH key authentication (no password prompt for SSH connections)

## Troubleshooting

### "No such host is known" error
- Ensure the site name matches your SSH config or use FQDN
- Test with: `ssh sitename` manually first

### "Permission denied" error
- Verify SSH key authentication is working
- Check that your key is added to the remote server's authorized_keys

### Build errors
- Ensure .NET 8.0 SDK is installed: `dotnet --version`
- Run `dotnet restore` to restore NuGet packages

### Runtime errors about dependencies
- If using framework-dependent build, ensure .NET runtime is installed
- Use self-contained build to avoid runtime dependencies

## Development

Run in development mode:
```bash
dotnet run
```

Run with arguments:
```bash
dotnet run -- --verbose site1
```

Run tests (when tests are added):
```bash
dotnet test
```

## File Size Comparison

- Perl version: ~4KB
- .NET framework-dependent: ~10KB + runtime
- .NET self-contained single file: ~75-80MB (includes entire runtime)

The self-contained version is larger but has zero dependencies and is easier to deploy.
