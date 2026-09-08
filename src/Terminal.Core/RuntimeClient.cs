using System.IO.Pipes;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace Paneacea.Core;

public sealed class RuntimeClient(string? pipeName = null)
{
    public static readonly JsonSerializerOptions JsonOptions = new() { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };
    public string PipeName { get; } = pipeName ?? "paneacea-" + string.Concat(Environment.UserName.Select(c => char.IsAsciiLetterOrDigit(c) ? c : '_'));
    public async Task<NamedPipeClientStream> Connect(CancellationToken cancellationToken = default)
    {
        var pipe = new NamedPipeClientStream(".", PipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
        try
        {
            await pipe.ConnectAsync(3000, cancellationToken);
            return pipe;
        }
        catch (Exception error)
        {
            AppLogger.Error("ipc.connect.failed", $"pipe={PipeName} error={error.Message}");
            await pipe.DisposeAsync();
            throw;
        }
    }
    public static async Task Send(Stream stream, object value, CancellationToken cancellationToken = default)
    {
        var bytes = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(value, JsonOptions) + "\n");
        if (bytes.Length > 1_048_576) throw new InvalidOperationException("Request exceeds 1 MiB.");
        await stream.WriteAsync(bytes, cancellationToken);
        await stream.FlushAsync(cancellationToken);
    }
    public static JsonNode Result(string? line)
    {
        var response = JsonNode.Parse(line ?? throw new IOException("Runtime disconnected."))!;
        if (response["ok"]?.GetValue<bool>() != true) throw new IOException(response["error"]?.GetValue<string>() ?? "Runtime request failed.");
        return response["result"]!;
    }
    public async Task<JsonNode> Call(string method, object? parameters = null, CancellationToken cancellationToken = default)
    {
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeout.CancelAfter(TimeSpan.FromSeconds(10));
        if (method is not ("pane.sendInput" or "state.get")) AppLogger.Info("ipc.call.start", $"method={method}");
        try
        {
            await using var pipe = await Connect(timeout.Token);
            await Send(pipe, new { id = Guid.NewGuid().ToString(), method, @params = parameters ?? new { } }, timeout.Token);
            using var reader = new StreamReader(pipe, Encoding.UTF8);
            var result = Result(await reader.ReadLineAsync(timeout.Token));
            if (method is not ("pane.sendInput" or "state.get")) AppLogger.Info("ipc.call.complete", $"method={method}");
            return result;
        }
        catch (Exception error)
        {
            AppLogger.Error("ipc.call.failed", $"method={method} error={error.Message}");
            throw;
        }
    }
    public async Task<WorkspaceState> State() => (await Call("state.get")).Deserialize<WorkspaceState>(JsonOptions)!;
}
