using System.IO;
using System.IO.Pipes;
using System.Reflection;
using System.Text;
using System.Text.Json.Nodes;
using System.Windows.Input;
using System.Xml.Linq;
using Paneacea;
using Paneacea.Core;

var pipeName = $"paneacea-connection-test-{Guid.NewGuid()}";
var report = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
using var connection = new PipeTerminalConnection(new RuntimeClient(pipeName), "test-pane", _ => report.TrySetResult());
{
    await using var resizeServer = Server(pipeName);
    var resizeConnection = resizeServer.WaitForConnectionAsync();

    connection.Start();
    await Task.Delay(100);
    Check(!resizeConnection.IsCompleted, "connection waits for an initial terminal size");

    connection.Resize(40, 100);
    await resizeConnection.WaitAsync(TimeSpan.FromSeconds(5));
    var resize = await Request(resizeServer);
    Check(resize["method"]?.GetValue<string>() == "terminal.resize", "initial resize is sent before attach");
    Check(resize["params"]?["rows"]?.GetValue<uint>() == 40, "initial row count is preserved");
    Check(resize["params"]?["columns"]?.GetValue<uint>() == 100, "initial column count is preserved");
    await Respond(resizeServer, resize, new JsonObject());
}
{
    await using var attachServer = Server(pipeName);
    await attachServer.WaitForConnectionAsync().WaitAsync(TimeSpan.FromSeconds(5));
    var attach = await Request(attachServer);
    Check(attach["method"]?.GetValue<string>() == "terminal.attach", "snapshot attaches after initial resize");
    await Respond(attachServer, attach, new JsonObject
    {
        ["data"] = "",
        ["exited"] = true,
        ["restored"] = true,
        ["viewportOffset"] = 0
    });
}
await report.Task.WaitAsync(TimeSpan.FromSeconds(5));

var modifierState = typeof(MainWindow).GetMethod("ModifierState", BindingFlags.Static | BindingFlags.NonPublic)!;
Check((ModifierKeys)modifierState.Invoke(null, [false, false, false])! == ModifierKeys.None, "plain arrows have no shortcut modifiers");
Check(
    (ModifierKeys)modifierState.Invoke(null, [false, true, true])! == (ModifierKeys.Alt | ModifierKeys.Shift),
    "pane resize requires Alt and Shift");
var hasShortcutModifier = typeof(MainWindow).GetMethod("HasShortcutModifier", BindingFlags.Static | BindingFlags.NonPublic)!;
Check(!(bool)hasShortcutModifier.Invoke(null, [false, false, false])!, "basic keys bypass Paneacea unconditionally");
Check((bool)hasShortcutModifier.Invoke(null, [true, false, false])!, "modified keys can be evaluated as shortcuts");
var pressedModifierState = typeof(MainWindow).GetMethod("PressedModifierState", BindingFlags.Static | BindingFlags.NonPublic)!;
Check(
    (ModifierKeys)pressedModifierState.Invoke(null, [new HashSet<int>()])! == ModifierKeys.None,
    "released modifiers cannot affect basic keys");
Check(
    (ModifierKeys)pressedModifierState.Invoke(null, [new HashSet<int> { 0x12, 0x10 }])! == (ModifierKeys.Alt | ModifierKeys.Shift),
    "pressed Alt and Shift enable pane resize shortcuts");
var window = XDocument.Load(Path.Combine(Environment.CurrentDirectory, "src", "TerminalApp", "MainWindow.xaml")).Root!;
Check(window.Attribute("Width")?.Value == "2100", "default window width is 1.75 times the original");
Check(window.Attribute("Height")?.Value == "1330", "default window height is 1.75 times the original");
Check(window.Attribute("WindowStartupLocation")?.Value == "CenterScreen", "default window is centered on screen");

Console.WriteLine("PASS terminal connection initial resize ordering");

static NamedPipeServerStream Server(string pipeName) => new(
    pipeName,
    PipeDirection.InOut,
    1,
    PipeTransmissionMode.Byte,
    PipeOptions.Asynchronous);

static async Task<JsonNode> Request(Stream stream)
{
    using var reader = new StreamReader(stream, Encoding.UTF8, leaveOpen: true);
    return JsonNode.Parse(await reader.ReadLineAsync() ?? throw new IOException("Client disconnected."))!;
}

static Task Respond(Stream stream, JsonNode request, JsonNode result) => RuntimeClient.Send(
    stream,
    new { id = request["id"]?.GetValue<string>(), ok = true, result });

static void Check(bool condition, string message)
{
    if (!condition) throw new Exception($"FAIL {message}");
}
