using System.Text.Json;
using System.Text.Json.Serialization;

namespace Paneacea.Core;

public sealed class WorkspaceState
{
    public List<Workspace> Workspaces { get; set; } = [];
    public Dictionary<string, Pane> Panes { get; set; } = [];
    public string? ActiveWorkspaceId { get; set; }
    public Dictionary<string, JsonElement> Settings { get; set; } = [];
}
public sealed class Workspace
{
    public string Id { get; set; } = "";
    public string Name { get; set; } = "";
    public string RootDirectory { get; set; } = "";
    public List<Tab> Tabs { get; set; } = [];
    public string? ActiveTabId { get; set; }
    public override string ToString() => Name;
}
public sealed class Tab
{
    public string Id { get; set; } = "";
    public string WorkspaceId { get; set; } = "";
    public string Title { get; set; } = "";
    public string ActivePaneId { get; set; } = "";
    public LayoutNode RootLayoutNode { get; set; } = new();
}
public sealed class Pane
{
    public string Id { get; set; } = "";
    public string Executable { get; set; } = "";
    public string Title { get; set; } = "";
    public List<string> Arguments { get; set; } = [];
    public string CurrentWorkingDirectory { get; set; } = "";
    public string? Error { get; set; }
}
public sealed class LayoutNode
{
    public string Type { get; set; } = "pane";
    public string? PaneId { get; set; }
    public string? Orientation { get; set; }
    public double Ratio { get; set; } = .5;
    public LayoutNode? First { get; set; }
    public LayoutNode? Second { get; set; }
}
public readonly record struct PaneBounds(string Id, double X, double Y, double Width, double Height);
public static class LayoutGeometry
{
    public static List<PaneBounds> Measure(LayoutNode node, double x = 0, double y = 0, double width = 1, double height = 1)
    {
        if (node.Type == "pane") return [new(node.PaneId!, x, y, width, height)];
        return node.Orientation == "vertical"
            ? [.. Measure(node.First!, x, y, width * node.Ratio, height), .. Measure(node.Second!, x + width * node.Ratio, y, width * (1 - node.Ratio), height)]
            : [.. Measure(node.First!, x, y, width, height * node.Ratio), .. Measure(node.Second!, x, y + height * node.Ratio, width, height * (1 - node.Ratio))];
    }
    public static string? Neighbor(LayoutNode node, string paneId, string direction)
    {
        var bounds = Measure(node);
        var current = bounds.FirstOrDefault(p => p.Id == paneId);
        if (current.Id is null) return null;
        var cx = current.X + current.Width / 2;
        var cy = current.Y + current.Height / 2;
        return bounds.Where(p => p.Id != paneId).Select(p => new { p.Id, Dx = p.X + p.Width / 2 - cx, Dy = p.Y + p.Height / 2 - cy })
            .Where(p => direction switch { "Left" => p.Dx < -.001, "Right" => p.Dx > .001, "Up" => p.Dy < -.001, "Down" => p.Dy > .001, _ => false })
            .OrderBy(p => direction is "Left" or "Right" ? Math.Abs(p.Dx) + 3 * Math.Abs(p.Dy) : Math.Abs(p.Dy) + 3 * Math.Abs(p.Dx))
            .Select(p => p.Id).FirstOrDefault();
    }
    public static (int[] Path, double Ratio)? Resize(LayoutNode node, string paneId, string direction, int[]? path = null)
    {
        path ??= [];
        if (node.Type == "pane") return null;
        var first = Measure(node.First!).Any(p => p.Id == paneId);
        if (!first && !Measure(node.Second!).Any(p => p.Id == paneId)) return null;
        var nested = Resize(first ? node.First! : node.Second!, paneId, direction, [.. path, first ? 0 : 1]);
        if (nested is not null) return nested;
        if ((direction is "Left" or "Right") != (node.Orientation == "vertical")) return null;
        return (path, Math.Clamp(node.Ratio + (direction is "Right" or "Down" ? .05 : -.05), .1, .9));
    }
}
