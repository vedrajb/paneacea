using System.Text.Json;
using TinkerShell.Core;

var tests = new (string Name, Action Test)[]
{
    ("Nested layout and directional focus", () =>
    {
        var layout = JsonSerializer.Deserialize<LayoutNode>("""
        {"type":"split","orientation":"vertical","ratio":0.6,"first":{"type":"pane","paneId":"a"},"second":{"type":"split","orientation":"horizontal","ratio":0.5,"first":{"type":"pane","paneId":"b"},"second":{"type":"pane","paneId":"c"}}}
        """, RuntimeClient.JsonOptions)!;
        Check(LayoutGeometry.Measure(layout).Count == 3);
        Check(LayoutGeometry.Measure(layout)[1].X == .6);
        Check(LayoutGeometry.Neighbor(layout, "b", "Down") == "c");
        Check(LayoutGeometry.Neighbor(layout, "c", "Left") == "a");
        Check(LayoutGeometry.Neighbor(layout, "a", "Left") is null);
        var resize = LayoutGeometry.Resize(layout, "c", "Down")!.Value;
        Check(resize.Path.SequenceEqual(new[] { 1 }));
        Check(Math.Abs(resize.Ratio - .55) < .001);
        Check(LayoutGeometry.Resize(layout, "missing", "Down") is null);
    }),
    ("Protocol errors are surfaced", () =>
    {
        Check(RuntimeClient.Result("{\"ok\":true,\"result\":{\"name\":\"你好\"}}")["name"]!.GetValue<string>() == "你好");
        var failed = false;
        try { RuntimeClient.Result("{\"ok\":false,\"error\":\"invalid pane\"}"); }
        catch (IOException error) { failed = error.Message == "invalid pane"; }
        Check(failed);
    }),
    ("Workspace protocol deserialization", () =>
    {
        var state = JsonSerializer.Deserialize<WorkspaceState>("""
        {"activeWorkspaceId":"w","workspaces":[{"id":"w","name":"Test","rootDirectory":"C:\\dev","tabs":[],"activeTabId":null}],"panes":{},"settings":{}}
        """, RuntimeClient.JsonOptions)!;
        Check(state.ActiveWorkspaceId == "w");
        Check(state.Workspaces[0].RootDirectory == @"C:\dev");
    }),
    ("Shell discovery exposes supported profiles", () =>
    {
        var shells = ShellDiscovery.Detect();
        Check(shells.Select(shell => shell.Id).SequenceEqual(new[] { "pwsh", "powershell", "git-bash", "cmd" }));
        Check(shells.All(shell => !string.IsNullOrWhiteSpace(shell.Executable)));
        Check(shells.Single(shell => shell.Id == "pwsh").Arguments.SequenceEqual(new[] { "-NoLogo" }));
        Check(shells.Single(shell => shell.Id == "powershell").Arguments.SequenceEqual(new[] { "-NoLogo" }));
        Check(shells.Single(shell => shell.Id == "git-bash").Arguments.SequenceEqual(new[] { "--login", "-i" }));
        if (OperatingSystem.IsWindows())
        {
            Check(shells.Single(shell => shell.Id == "powershell").IsAvailable);
            Check(shells.Single(shell => shell.Id == "cmd").IsAvailable);
        }
    })
};
foreach (var (name, test) in tests) { test(); Console.WriteLine($"PASS {name}"); }
Console.WriteLine($"{tests.Length} tests passed.");
static void Check(bool condition) { if (!condition) throw new Exception("Assertion failed."); }
