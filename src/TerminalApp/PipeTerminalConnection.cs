using Microsoft.Terminal.Wpf;
using System.IO;
using System.Text;
using System.Text.Json.Nodes;
using System.Threading.Channels;
using Paneacea.Core;

namespace Paneacea;

public sealed class PipeTerminalConnection(RuntimeClient client, string paneId, Action<string> report) : ITerminalConnection, IDisposable
{
    public event EventHandler<TerminalOutputEventArgs>? TerminalOutput;
    private readonly CancellationTokenSource cancellation = new();
    private readonly Channel<(string Method, object Parameters)> commands = Channel.CreateBounded<(string, object)>(512);
    private int started;
    private int disposed;
    public void Start()
    {
        if (Interlocked.Exchange(ref started, 1) != 0) return;
        _ = Read();
        _ = Write();
    }
    private async Task Read()
    {
        try
        {
            await using var pipe = await client.Connect(cancellation.Token);
            await RuntimeClient.Send(pipe, new { id = "attach", method = "terminal.attach", @params = new { paneId } }, cancellation.Token);
            using var reader = new StreamReader(pipe, Encoding.UTF8);
            var result = RuntimeClient.Result(await reader.ReadLineAsync(cancellation.Token));
            var decoder = Encoding.UTF8.GetDecoder();
            void Output(string? data)
            {
                if (data is null) return;
                var bytes = Convert.FromBase64String(data);
                var chars = new char[Encoding.UTF8.GetMaxCharCount(bytes.Length)];
                var count = decoder.GetChars(bytes, chars, false);
                if (count > 0) TerminalOutput?.Invoke(this, new TerminalOutputEventArgs(new string(chars, 0, count)));
            }
            Output(result["data"]?.GetValue<string>());
            if (result["exited"]?.GetValue<bool>() == true) { report("Shell exited. Close this pane or open a new tab."); return; }
            while (await reader.ReadLineAsync(cancellation.Token) is { } line)
            {
                var message = JsonNode.Parse(line)!;
                if (message["event"]?.GetValue<string>() == "terminal.exited") { report("Shell exited. Close this pane or open a new tab."); return; }
                Output(message["data"]?.GetValue<string>());
            }
            report("Terminal disconnected. Use Reconnect in Commands.");
        }
        catch (OperationCanceledException) { }
        catch (Exception error) { report(error.Message); }
    }
    private async Task Write()
    {
        try
        {
            await foreach (var command in commands.Reader.ReadAllAsync(cancellation.Token))
                await client.Call(command.Method, command.Parameters, cancellation.Token);
        }
        catch (OperationCanceledException) { }
        catch (Exception error) { report(error.Message); }
    }
    private void Queue(string method, object parameters)
    {
        if (Volatile.Read(ref disposed) != 0) return;
        if (!commands.Writer.TryWrite((method, parameters))) report("Terminal input queue is full; wait before typing again.");
    }
    public void WriteInput(string data) => Queue("pane.sendInput", new { paneId, data });
    public void Resize(uint rows, uint columns) { if (rows > 0 && columns > 0) Queue("terminal.resize", new { paneId, rows, columns }); }
    public void Close() => Dispose();
    public void Dispose()
    {
        if (Interlocked.Exchange(ref disposed, 1) != 0) return;
        commands.Writer.TryComplete(); cancellation.Cancel();
    }
}
