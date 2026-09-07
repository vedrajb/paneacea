using System.Text.Json.Serialization;

namespace TinkerShell.Core;

public sealed class ShellProfile
{
    public string Id { get; init; } = "";
    public string Name { get; init; } = "";
    public string Executable { get; init; } = "";
    public List<string> Arguments { get; init; } = [];
    public string Description { get; init; } = "";
    [JsonIgnore]
    public bool IsAvailable { get; init; }
}

public static class ShellDiscovery
{
    public static IReadOnlyList<ShellProfile> Detect()
    {
        var pwsh = FindOnPath("pwsh.exe");
        var powershell = OperatingSystem.IsWindows()
            ? ExistingPath(Path.Combine(Environment.SystemDirectory, "WindowsPowerShell", "v1.0", "powershell.exe")) ?? FindOnPath("powershell.exe")
            : FindOnPath("powershell.exe");
        var gitBash = FindGitBash();
        var cmd = OperatingSystem.IsWindows()
            ? ExistingPath(Path.Combine(Environment.SystemDirectory, "cmd.exe")) ?? FindOnPath("cmd.exe")
            : FindOnPath("cmd.exe");

        return
        [
            new ShellProfile
            {
                Id = "pwsh",
                Name = "PowerShell 7",
                Executable = pwsh ?? "pwsh.exe",
                Arguments = ["-NoLogo"],
                Description = "PowerShell 7 (pwsh)",
                IsAvailable = pwsh is not null
            },
            new ShellProfile
            {
                Id = "powershell",
                Name = "Windows PowerShell",
                Executable = powershell ?? "powershell.exe",
                Arguments = ["-NoLogo"],
                Description = "Windows PowerShell 5.1",
                IsAvailable = powershell is not null
            },
            new ShellProfile
            {
                Id = "git-bash",
                Name = "Git Bash",
                Executable = gitBash ?? "bash.exe",
                Arguments = ["--login", "-i"],
                Description = "Bash from Git for Windows",
                IsAvailable = gitBash is not null
            },
            new ShellProfile
            {
                Id = "cmd",
                Name = "Command Prompt",
                Executable = cmd ?? "cmd.exe",
                Arguments = [],
                Description = "Windows Command Prompt",
                IsAvailable = cmd is not null
            }
        ];
    }

    public static IReadOnlyList<ShellProfile> GetAvailable() => Detect().Where(shell => shell.IsAvailable).ToArray();

    private static string? FindGitBash()
    {
        var fromPath = FindOnPath("bash.exe");
        if (IsGitBash(fromPath)) return fromPath;

        var roots = new List<string>();
        AddGitRoot(roots, Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles));
        AddGitRoot(roots, Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86));
        AddGitRoot(roots, Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData));

        var git = FindOnPath("git.exe");
        if (git is not null)
        {
            var directory = new DirectoryInfo(Path.GetDirectoryName(git)!);
            for (var depth = 0; depth < 4 && directory is not null; depth++, directory = directory.Parent!)
                if (directory.Name.Equals("Git", StringComparison.OrdinalIgnoreCase)) roots.Add(directory.FullName);
        }

        foreach (var root in roots.Distinct(StringComparer.OrdinalIgnoreCase))
            foreach (var relative in new[] { Path.Combine("usr", "bin", "bash.exe"), Path.Combine("bin", "bash.exe") })
                if (ExistingPath(Path.Combine(root, relative)) is { } candidate) return candidate;
        return null;
    }

    private static void AddGitRoot(List<string> roots, string parent)
    {
        if (!string.IsNullOrWhiteSpace(parent)) roots.Add(Path.Combine(parent, "Git"));
    }

    private static bool IsGitBash(string? path)
    {
        if (path is null) return false;
        var normalized = path.Replace('/', '\\');
        return normalized.Contains("\\Git\\", StringComparison.OrdinalIgnoreCase)
            && (normalized.EndsWith("\\usr\\bin\\bash.exe", StringComparison.OrdinalIgnoreCase)
                || normalized.EndsWith("\\bin\\bash.exe", StringComparison.OrdinalIgnoreCase));
    }

    private static string? FindOnPath(string fileName)
    {
        var path = Environment.GetEnvironmentVariable("PATH");
        if (path is null) return null;
        foreach (var directory in path.Split(Path.PathSeparator))
        {
            if (string.IsNullOrWhiteSpace(directory)) continue;
            var candidate = ExistingPath(Path.Combine(directory.Trim().Trim('"'), fileName));
            if (candidate is not null) return candidate;
        }
        return null;
    }

    private static string? ExistingPath(string path) => File.Exists(path) ? Path.GetFullPath(path) : null;
}
