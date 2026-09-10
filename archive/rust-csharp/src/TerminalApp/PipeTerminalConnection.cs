using Microsoft.Terminal.Wpf;
using System.IO;
using System.Text;
using System.Text.Json.Nodes;
using System.Threading.Channels;
using Paneacea.Core;

namespace Paneacea;

public sealed class PipeTerminalConnection(RuntimeClient client, string paneId, Action<string> report) : ITerminalConnection, IDisposable
{
    public string PaneId => paneId;
    public event EventHandler<TerminalOutputEventArgs>? TerminalOutput;
    public event Action<ulong>? ViewportRestored;
    public ulong? RestoredViewport { get; private set; }
    private readonly CancellationTokenSource cancellation = new();
    private readonly Channel<(string Method, object Parameters)> commands = Channel.CreateBounded<(string, object)>(512);
    private readonly TaskCompletionSource initialSize = new(TaskCreationOptions.RunContinuationsAsynchronously);
    private readonly object sizeLock = new();
    private uint latestRows;
    private uint latestColumns;
    private int started;
    private int disposed;
    private int attached;
    public void Start()
    {
        if (Interlocked.Exchange(ref started, 1) != 0) return;
        AppLogger.Info("terminal.connection.start", $"pane_id={paneId}");
        _ = Read();
        _ = Write();
    }
    private async Task Read()
    {
        try
        {
            await initialSize.Task.WaitAsync(cancellation.Token);
            while (true)
            {
                uint rows;
                uint columns;
                lock (sizeLock) (rows, columns) = (latestRows, latestColumns);
                AppLogger.Info("terminal.initial_resize.start", $"pane_id={paneId} rows={rows} columns={columns}");
                await client.Call("terminal.resize", new { paneId, rows, columns }, cancellation.Token);
                AppLogger.Info("terminal.initial_resize.complete", $"pane_id={paneId} rows={rows} columns={columns}");
                lock (sizeLock)
                {
                    if (rows != latestRows || columns != latestColumns) continue;
                    Volatile.Write(ref attached, 1);
                    break;
                }
            }
            await using var pipe = await client.Connect(cancellation.Token);
            AppLogger.Info("terminal.attach.start", $"pane_id={paneId}");
            await RuntimeClient.Send(pipe, new { id = "attach", method = "terminal.attach", @params = new { paneId } }, cancellation.Token);
            using var reader = new StreamReader(pipe, Encoding.UTF8);
            var result = RuntimeClient.Result(await reader.ReadLineAsync(cancellation.Token));
            var snapshotBytes = result["data"]?.GetValue<string>() is { } snapshot ? Convert.FromBase64String(snapshot).Length : 0;
            AppLogger.Info(
                "terminal.attach.complete",
                $"pane_id={paneId} snapshot_bytes={snapshotBytes} exited={result["exited"]?.GetValue<bool>() ?? false} restored={result["restored"]?.GetValue<bool>() ?? false} viewport={result["viewportOffset"]?.ToString() ?? "none"}");
            var decoder = Encoding.UTF8.GetDecoder();
            var outputEvents = 0;
            var outputBytes = 0;
            void Output(string? data)
            {
                if (data is null) return;
                var bytes = Convert.FromBase64String(data);
                outputEvents++;
                outputBytes += bytes.Length;
                if (outputEvents <= 3 || outputEvents % 100 == 0)
                    AppLogger.Info("terminal.output", $"pane_id={paneId} event={outputEvents} bytes={bytes.Length} total_bytes={outputBytes}");
                var chars = new char[Encoding.UTF8.GetMaxCharCount(bytes.Length)];
                var count = decoder.GetChars(bytes, chars, false);
                if (count > 0) TerminalOutput?.Invoke(this, new TerminalOutputEventArgs(new string(chars, 0, count)));
            }
            Output(result["data"]?.GetValue<string>());
            if (result["viewportOffset"] is JsonValue viewport && viewport.TryGetValue<long>(out var offset) && offset >= 0)
            {
                RestoredViewport = (ulong)offset;
                ViewportRestored?.Invoke(RestoredViewport.Value);
            }
            if (result["exited"]?.GetValue<bool>() == true)
            {
                AppLogger.Info("terminal.exited", $"pane_id={paneId} output_events={outputEvents} output_bytes={outputBytes}");
                report("Shell exited. Close this pane or open a new tab.");
                return;
            }
            while (await reader.ReadLineAsync(cancellation.Token) is { } line)
            {
                var message = JsonNode.Parse(line)!;
                if (message["event"]?.GetValue<string>() == "terminal.exited")
                {
                    AppLogger.Info("terminal.exited", $"pane_id={paneId} output_events={outputEvents} output_bytes={outputBytes}");
                    report("Shell exited. Close this pane or open a new tab.");
                    return;
                }
                Output(message["data"]?.GetValue<string>());
            }
            AppLogger.Info("terminal.disconnected", $"pane_id={paneId} output_events={outputEvents} output_bytes={outputBytes}");
            report("Terminal disconnected. Use Reconnect in Commands.");
        }
        catch (OperationCanceledException) { }
        catch (Exception error)
        {
            AppLogger.Error("terminal.read.failed", $"pane_id={paneId} error={error.Message}");
            report(error.Message);
        }
    }
    private async Task Write()
    {
        try
        {
            await foreach (var command in commands.Reader.ReadAllAsync(cancellation.Token))
                await client.Call(command.Method, command.Parameters, cancellation.Token);
        }
        catch (OperationCanceledException) { }
        catch (Exception error)
        {
            AppLogger.Error("terminal.write.failed", $"pane_id={paneId} error={error.Message}");
            report(error.Message);
        }
    }
    private void Queue(string method, object parameters)
    {
        if (Volatile.Read(ref disposed) != 0) return;
        if (!commands.Writer.TryWrite((method, parameters)))
        {
            AppLogger.Error("terminal.command_queue.full", $"pane_id={paneId} method={method}");
            report("Terminal input queue is full; wait before typing again.");
        }
    }
    public void WriteInput(string data) => Queue("pane.sendInput", new { paneId, data });
    public void Resize(uint rows, uint columns)
    {
        if (rows == 0 || columns == 0) return;
        lock (sizeLock)
        {
            latestRows = rows;
            latestColumns = columns;
            if (Volatile.Read(ref attached) == 0)
            {
                initialSize.TrySetResult();
                return;
            }
        }
        Queue("terminal.resize", new { paneId, rows, columns });
    }
    public void SetViewport(ulong offset) => Queue("terminal.viewport.set", new { paneId, offset });
    public void Close() => Dispose();
    public void Dispose()
    {
        if (Interlocked.Exchange(ref disposed, 1) != 0) return;
        AppLogger.Info("terminal.connection.dispose", $"pane_id={paneId}");
        commands.Writer.TryComplete(); cancellation.Cancel();
    }
}
