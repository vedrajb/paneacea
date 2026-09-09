using System.Diagnostics;
using System.Text;

namespace Paneacea.Core;

public static class AppLogger
{
    private static readonly object Gate = new();
    private static string? path;

    public static string DataDirectory
    {
        get
        {
            var dataDirectory = Environment.GetEnvironmentVariable("PANEACEA_DATA");
            return string.IsNullOrWhiteSpace(dataDirectory) ? AppContext.BaseDirectory : dataDirectory;
        }
    }

    public static string LogPath
    {
        get
        {
            lock (Gate)
                return InitializeLocked();
        }
    }

    public static void Initialize(string? preferredPath = null)
    {
        lock (Gate)
            InitializeLocked(preferredPath);
    }

    public static void Info(string eventName, string details = "") => Write("INFO", eventName, details);

    public static void Error(string eventName, string details = "") => Write("ERROR", eventName, details);

    private static string InitializeLocked(string? preferredPath = null)
    {
        if (path is not null) return path;
        var configuredPath = preferredPath ?? Environment.GetEnvironmentVariable("PANEACEA_LOG");
        var dataDirectory = DataDirectory;
        path = !string.IsNullOrWhiteSpace(configuredPath)
            ? configuredPath
            : Path.Combine(
                dataDirectory,
                "logs",
                "paneacea-app.log");
        try
        {
            var directory = Path.GetDirectoryName(path);
            if (!string.IsNullOrWhiteSpace(directory)) Directory.CreateDirectory(directory);
            using var stream = new FileStream(path, FileMode.OpenOrCreate, FileAccess.Write, FileShare.ReadWrite);
            stream.Seek(0, SeekOrigin.End);
        }
        catch (Exception error)
        {
            Debug.WriteLine($"Paneacea logging initialization failed: {error.Message}");
        }
        return path;
    }

    private static void Write(string level, string eventName, string details)
    {
        var timestamp = DateTimeOffset.UtcNow.ToString("O");
        var line = $"{timestamp} pid={Environment.ProcessId} [{level}] {eventName} {details.Replace('\r', ' ').Replace('\n', ' ')}";
        Debug.WriteLine(line);
        lock (Gate)
        {
            var target = InitializeLocked();
            try
            {
                using var stream = new FileStream(target, FileMode.Append, FileAccess.Write, FileShare.ReadWrite);
                using var writer = new StreamWriter(stream, new UTF8Encoding(false));
                writer.WriteLine(line);
            }
            catch (Exception error)
            {
                Debug.WriteLine($"Paneacea logging write failed: {error.Message}");
            }
        }
    }
}
