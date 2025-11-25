using System.CommandLine;
using System.Text;
using System.Text.Json;
using Renci.SshNet;

namespace AideChecker;

class Program
{
    private const int NRETRY = 3;
    private const string AIDE_HOME = "/var/lib/aide";
    private const string AIDE_EXE = "/usr/sbin/aide";

    private static bool _verbose = false;

    static async Task<int> Main(string[] args)
    {
        var dbOption = new Option<string>(
            name: "--db",
            description: "Database directory path",
            getDefaultValue: () => Path.Combine(AppContext.BaseDirectory, "db"));

        var updateOption = new Option<string?>(
            name: "--update",
            description: "Update AIDE database for specified site");

        var mailOption = new Option<string?>(
            name: "--mail",
            description: "Email address for notifications");

        var verboseOption = new Option<bool>(
            name: "--verbose",
            description: "Enable verbose output");

        var sitesArgument = new Argument<string[]>(
            name: "sites",
            description: "Specific sites to check",
            getDefaultValue: () => Array.Empty<string>());

        var rootCommand = new RootCommand("AIDE checker - Run AIDE integrity checks on remote servers")
        {
            dbOption,
            updateOption,
            mailOption,
            verboseOption,
            sitesArgument
        };

        rootCommand.SetHandler(async (db, update, mail, verbose, sites) =>
        {
            _verbose = verbose;

            if (update != null)
            {
                var who = Environment.GetEnvironmentVariable("USER") ?? "unknown";
                await UpdateSite(update, db, who);
            }
            else
            {
                await CheckSites(db, sites, mail);
            }
        }, dbOption, updateOption, mailOption, verboseOption, sitesArgument);

        return await rootCommand.InvokeAsync(args);
    }

    static async Task CheckSites(string dbDir, string[] selectedSites, string? mail)
    {
        if (!Directory.Exists(dbDir))
        {
            Console.Error.WriteLine($"Database directory not found: {dbDir}");
            Environment.Exit(1);
        }

        var allSites = Directory.GetDirectories(dbDir)
            .Select(Path.GetFileName)
            .Where(name => name != null && !name.StartsWith('.'))
            .Select(name => name!)
            .OrderBy(name => name)
            .ToList();

        var sites = selectedSites.Length > 0
            ? allSites.Where(s => selectedSites.Contains(s)).ToList()
            : allSites;

        var passed = new List<string>();
        var failed = new List<string>();

        foreach (var site in sites)
        {
            if (await CheckSite(site, dbDir))
            {
                passed.Add(site);
            }
            else
            {
                failed.Add(site);
            }
        }

        var message = GetMessage(passed, failed);

        if (!string.IsNullOrEmpty(mail))
        {
            await SendEmail(mail, message);
        }

        Console.WriteLine(message);
    }

    static async Task<bool> CheckSite(string site, string dbDir)
    {
        int retry = NRETRY;

        while (retry >= 0)
        {
            try
            {
                Console.WriteLine($"################################## Checking {site}");

                var result = await CheckSiteInternal(site, dbDir);

                Console.WriteLine($"################################## Finished {site}");

                return result;
            }
            catch (Exception ex)
            {
                Console.Error.WriteLine($"Error: {ex.Message}");
                if (retry > 0)
                {
                    var sleepTime = Random.Shared.Next(60000);
                    await Task.Delay(sleepTime);
                }
            }

            retry--;
        }

        return false;
    }

    static async Task<bool> CheckSiteInternal(string site, string dbDir)
    {
        var config = GetConfig(site, dbDir);
        return await RunRemoteCommand(site, config.Check, ignoreErrors: false);
    }

    static async Task UpdateSite(string site, string dbDir, string who)
    {
        var config = GetConfig(site, dbDir);

        Console.WriteLine($"Updating {site}");

        var updateCommands = config.Update;

        // Check if any command needs sudo password
        bool needsSudoPassword = updateCommands.Any(cmd => cmd.Contains("{SUDO_PW}"));
        string? sudoPassword = null;

        if (needsSudoPassword)
        {
            Console.Write("Enter sudo password: ");
            sudoPassword = ReadPassword();
            Console.WriteLine(); // New line after password input

            // Replace {SUDO_PW} placeholder
            updateCommands = updateCommands
                .Select(cmd => cmd.Replace("{SUDO_PW}", sudoPassword))
                .ToArray();
        }

        foreach (var command in updateCommands)
        {
            var success = await RunRemoteCommand(site, command, ignoreErrors: true);
            if (!success)
            {
                throw new Exception("Update command failed!");
            }
        }
    }

    static async Task<bool> RunRemoteCommand(string host, string command, bool ignoreErrors)
    {
        try
        {
            if (_verbose)
            {
                Console.WriteLine($"Running on {host}: {command}");
            }

            using var client = new SshClient(host, Environment.UserName);

            // Set connection options
            client.ConnectionInfo.Timeout = TimeSpan.FromSeconds(30);

            client.Connect();

            if (!client.IsConnected)
            {
                if (!ignoreErrors)
                {
                    Console.Error.WriteLine($"Failed to connect to {host}");
                }
                return ignoreErrors;
            }

            var cmd = client.RunCommand(command);

            if (_verbose)
            {
                if (!string.IsNullOrEmpty(cmd.Result))
                {
                    Console.WriteLine(cmd.Result);
                }
                if (!string.IsNullOrEmpty(cmd.Error))
                {
                    Console.Error.WriteLine(cmd.Error);
                }
            }

            client.Disconnect();

            if (cmd.ExitStatus != 0 && !ignoreErrors)
            {
                Console.Error.WriteLine($"Command failed with exit code {cmd.ExitStatus}");
                return false;
            }

            return true;
        }
        catch (Exception ex)
        {
            if (!ignoreErrors)
            {
                Console.Error.WriteLine($"SSH Error: {ex.Message}");
            }
            return ignoreErrors;
        }
    }

    static SiteConfig GetConfig(string site, string dbDir)
    {
        // Default configuration
        var config = new SiteConfig
        {
            Update = new[]
            {
                $"sudo {AIDE_EXE} --update",
                $"echo {{SUDO_PW}}|sudo -S mv {AIDE_HOME}/aide.db.new.gz {AIDE_HOME}/aide.db.gz"
            },
            Check = $"sudo {AIDE_EXE} --check"
        };

        // Check for site-specific config
        var configFile = Path.Combine(dbDir, site, "config.json");
        if (File.Exists(configFile))
        {
            try
            {
                var json = File.ReadAllText(configFile);
                var siteConfig = JsonSerializer.Deserialize<SiteConfigJson>(json, new JsonSerializerOptions
                {
                    PropertyNameCaseInsensitive = true
                });

                if (siteConfig != null)
                {
                    var updateCommands = siteConfig.GetUpdate();
                    if (updateCommands.Length > 0)
                    {
                        config.Update = updateCommands;
                    }
                    if (siteConfig.Check != null)
                    {
                        config.Check = siteConfig.Check;
                    }
                }
            }
            catch (Exception ex)
            {
                Console.Error.WriteLine($"Warning: Failed to parse config file {configFile}: {ex.Message}");
            }
        }

        return config;
    }

    static string GetMessage(List<string> passed, List<string> failed)
    {
        var passedStr = passed.Count > 0 ? string.Join(", ", passed) : "NONE";
        var failedStr = failed.Count > 0 ? string.Join(", ", failed) : "NONE";

        return $@"Passed AIDE checks: {passedStr}
Failed: {failedStr}

";
    }

    static async Task SendEmail(string emailAddress, string message)
    {
        try
        {
            var process = new System.Diagnostics.Process
            {
                StartInfo = new System.Diagnostics.ProcessStartInfo
                {
                    FileName = "mail",
                    Arguments = $"-s 'AIDE checks' {emailAddress}",
                    RedirectStandardInput = true,
                    UseShellExecute = false,
                    CreateNoWindow = true
                }
            };

            process.Start();
            await process.StandardInput.WriteAsync(message);
            process.StandardInput.Close();
            await process.WaitForExitAsync();
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine($"Failed to send email: {ex.Message}");
        }
    }

    static string ReadPassword()
    {
        var password = new StringBuilder();
        while (true)
        {
            var key = Console.ReadKey(intercept: true);
            if (key.Key == ConsoleKey.Enter)
            {
                break;
            }
            Console.Write("*");
            password.Append(key.KeyChar);
        }
        return password.ToString();
    }
}

class SiteConfig
{
    public string[] Update { get; set; } = Array.Empty<string>();
    public string Check { get; set; } = string.Empty;
}

class SiteConfigJson
{
    public JsonElement? Update { get; set; }
    public string? Check { get; set; }

    public string[] GetUpdate()
    {
        if (Update == null)
            return Array.Empty<string>();

        if (Update.Value.ValueKind == JsonValueKind.String)
        {
            var str = Update.Value.GetString();
            return str != null ? new[] { str } : Array.Empty<string>();
        }
        else if (Update.Value.ValueKind == JsonValueKind.Array)
        {
            return Update.Value.EnumerateArray()
                .Where(e => e.ValueKind == JsonValueKind.String)
                .Select(e => e.GetString()!)
                .ToArray();
        }

        return Array.Empty<string>();
    }
}
