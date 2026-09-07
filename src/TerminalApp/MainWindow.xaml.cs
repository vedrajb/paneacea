using Microsoft.Terminal.Wpf;
using System.Diagnostics;
using System.IO;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Interop;
using System.Windows.Media;
using TinkerShell.Core;

namespace TinkerShell;

public partial class MainWindow : Window
{
    private readonly RuntimeClient client = new(Environment.GetEnvironmentVariable("PANEACEA_PIPE") ?? Environment.GetEnvironmentVariable("TINKERSHELL_PIPE"));
    private WorkspaceState state = new();
    private readonly Dictionary<string, TerminalControl> controls = [];
    private readonly List<PipeTerminalConnection> connections = [];
    private bool updating;
    private bool busy;
    private bool dialog;
    private bool showingSettings;
    private IReadOnlyList<ShellProfile> shells = [];
    private Workspace? ActiveWorkspace => state.Workspaces.FirstOrDefault(w => w.Id == state.ActiveWorkspaceId);
    private Tab? ActiveTab => ActiveWorkspace?.Tabs.FirstOrDefault(t => t.Id == ActiveWorkspace.ActiveTabId);
    private Brush ThemeBrush(string key) => (Brush)FindResource(key);
    private string? ConfiguredShellId
    {
        get
        {
            if (!state.Settings.TryGetValue("defaultShell", out var setting)) return null;
            if (setting.ValueKind == JsonValueKind.String) return setting.GetString();
            if (setting.ValueKind == JsonValueKind.Object && setting.TryGetProperty("id", out var id)) return id.GetString();
            return null;
        }
    }
    private ShellProfile? ConfiguredShell => ConfiguredShellId is { } id ? shells.FirstOrDefault(shell => shell.Id.Equals(id, StringComparison.OrdinalIgnoreCase)) : null;
    private ShellProfile? DefaultShell => ConfiguredShell is { IsAvailable: true } configured
        ? configured
        : shells.FirstOrDefault(shell => shell.IsAvailable && shell.Id == "pwsh")
            ?? shells.FirstOrDefault(shell => shell.IsAvailable && shell.Id == "powershell")
            ?? shells.FirstOrDefault(shell => shell.IsAvailable && shell.Id == "git-bash")
            ?? shells.FirstOrDefault(shell => shell.IsAvailable);
    private static object ShellSetting(ShellProfile shell) => new { id = shell.Id, executable = shell.Executable, arguments = shell.Arguments };

    public MainWindow()
    {
        InitializeComponent();
        Loaded += async (_, _) => await Run(Initialize);
        ComponentDispatcher.ThreadPreprocessMessage += KeyMessage;
        StateChanged += (_, _) => MaximizeWindowButton.Content = WindowState == WindowState.Maximized ? "❐" : "□";
        Closed += (_, _) => { ComponentDispatcher.ThreadPreprocessMessage -= KeyMessage; Detach(); };
    }
    private async Task Run(Func<Task> action)
    {
        if (busy) return;
        busy = true;
        try { await action(); }
        catch (Exception error) { Report(error.Message); }
        finally { busy = false; }
    }
    private void Report(string message) => Dispatcher.BeginInvoke(() => Status.Text = message);
    private async Task Initialize()
    {
        try { state = await client.State(); }
        catch (Exception error) when (error is TimeoutException or IOException or OperationCanceledException)
        {
            var executable = Path.Combine(AppContext.BaseDirectory, "mux-runtime.exe");
            if (!File.Exists(executable)) throw new FileNotFoundException("Build with scripts/build.ps1 so mux-runtime.exe is beside Paneacea.App.exe.");
            var start = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = AppContext.BaseDirectory };
            start.ArgumentList.Add("--pipe"); start.ArgumentList.Add(client.PipeName);
            if ((Environment.GetEnvironmentVariable("PANEACEA_DATA") ?? Environment.GetEnvironmentVariable("TINKERSHELL_DATA")) is { Length: > 0 } directory) { start.ArgumentList.Add("--data"); start.ArgumentList.Add(directory); }
            using var process = Process.Start(start);
            for (var attempt = 0; ; attempt++)
            {
                try { state = await client.State(); break; }
                catch when (attempt < 4) { await Task.Delay(250); }
            }
        }
        await EnsureDefaultShell();
        if (state.Workspaces.Count == 0)
        {
            await Change("workspace.create", new { name = "Default", rootDirectory = Environment.CurrentDirectory }, false);
            await Change("tab.create", NewTabParameters(state.ActiveWorkspaceId!), false);
        }
        Render();
    }
    private async Task EnsureDefaultShell()
    {
        shells = ShellDiscovery.Detect();
        var selected = DefaultShell;
        if (selected is not null && !DefaultShellSettingMatches(selected))
            await Change("settings.set", new { key = "defaultShell", value = ShellSetting(selected) }, false);
    }
    private bool DefaultShellSettingMatches(ShellProfile shell)
    {
        if (!state.Settings.TryGetValue("defaultShell", out var setting) || setting.ValueKind != JsonValueKind.Object) return false;
        if (!setting.TryGetProperty("id", out var id) || !string.Equals(id.GetString(), shell.Id, StringComparison.OrdinalIgnoreCase)) return false;
        if (!setting.TryGetProperty("executable", out var executable) || !string.Equals(executable.GetString(), shell.Executable, StringComparison.OrdinalIgnoreCase)) return false;
        if (!setting.TryGetProperty("arguments", out var arguments) || arguments.ValueKind != JsonValueKind.Array) return false;
        var values = arguments.EnumerateArray().Select(argument => argument.GetString() ?? "").ToArray();
        return values.SequenceEqual(shell.Arguments);
    }
    private async Task Change(string method, object parameters, bool render = true)
    {
        state = (await client.Call(method, parameters)).Deserialize<WorkspaceState>(RuntimeClient.JsonOptions)!;
        if (render) Render();
    }
    private void Detach()
    {
        foreach (var connection in connections) connection.Dispose();
        connections.Clear(); controls.Clear(); PaneHost.Content = null;
    }
    private void Render()
    {
        updating = true;
        WorkspaceList.ItemsSource = state.Workspaces;
        WorkspaceList.SelectedItem = ActiveWorkspace;
        updating = false;
        TabStrip.Children.Clear();
        foreach (var tab in ActiveWorkspace?.Tabs ?? [])
        {
            var isActiveTab = tab.Id == ActiveTab?.Id;
            var button = new Button
            {
                Content = tab.Title,
                Background = isActiveTab ? ThemeBrush("TabActiveBackgroundBrush") : ThemeBrush("TabBackgroundBrush"),
                Foreground = ThemeBrush("ForegroundBrush"),
                BorderBrush = isActiveTab ? ThemeBrush("AccentBrush") : ThemeBrush("BorderBrush"),
                BorderThickness = new Thickness(0, 0, 0, 2),
                MinHeight = 40,
                Padding = new Thickness(14, 0, 14, 0),
                ToolTip = $"Terminal tab: {tab.Title}. Right-click to rename or close."
            };
            button.Click += async (_, _) => await Run(() => Change("tab.focus", new { tabId = tab.Id }));
            var menu = new ContextMenu();
            var rename = new MenuItem { Header = "Rename" };
            rename.Click += async (_, _) => await Run(async () => { if (Prompt("Rename tab", "Title", tab.Title) is { } title) await Change("tab.rename", new { tabId = tab.Id, title }); });
            var close = new MenuItem { Header = "Close tab" };
            close.Click += async (_, _) => await Run(() => Change("tab.close", new { tabId = tab.Id }));
            menu.Items.Add(rename); menu.Items.Add(close); button.ContextMenu = menu;
            TabStrip.Children.Add(button);
        }
        Detach();
        if (showingSettings)
            PaneHost.Content = BuildSettingsPanel();
        else if (ActiveTab is { } active)
        {
            PaneHost.Content = BuildLayout(active.RootLayoutNode, []);
            Dispatcher.BeginInvoke(() => { if (controls.TryGetValue(active.ActivePaneId, out var control)) control.Focus(); });
        }
        else PaneHost.Content = new TextBlock { Text = "Open a workspace folder or create a new terminal tab to start.", Margin = new Thickness(30), Foreground = ThemeBrush("MutedForegroundBrush") };
        Status.Text = ActiveWorkspace is { } workspace ? $"{workspace.Name}  ·  {workspace.RootDirectory}  ·  {DefaultShell?.Name ?? "No shell"}" : "No workspace open";
        RuntimeState.Text = "Runtime · Connected";
    }
    private UIElement BuildLayout(LayoutNode node, int[] path)
    {
        if (node.Type == "pane")
        {
            var pane = state.Panes[node.PaneId!];
            var panel = new DockPanel();
            var header = new Button
            {
                Content = $"TERMINAL  {Path.GetFileName(pane.Executable)}",
                HorizontalContentAlignment = HorizontalAlignment.Left,
                Padding = new Thickness(10, 3, 10, 3),
                Margin = new Thickness(0),
                Background = ThemeBrush("PanelBackgroundBrush"),
                Foreground = ThemeBrush("MutedForegroundBrush"),
                BorderBrush = ThemeBrush("BorderBrush"),
                BorderThickness = new Thickness(0, 0, 0, 1),
                ToolTip = pane.CurrentWorkingDirectory
            };
            DockPanel.SetDock(header, Dock.Top); panel.Children.Add(header);
            header.Click += async (_, _) => await Run(() => FocusPane(pane.Id));
            if (pane.Error is not null) { panel.Children.Add(new TextBlock { Text = pane.Error, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(15), Foreground = ThemeBrush("ErrorBrush") }); return panel; }
            var control = new TerminalControl { Focusable = true };
            var connection = new PipeTerminalConnection(client, pane.Id, Report);
            control.Connection = connection;
            controls[pane.Id] = control; connections.Add(connection);
            control.Loaded += (_, _) => control.SetTheme(new TerminalTheme
            {
                DefaultBackground = 0x1E1E1E, DefaultForeground = 0xCCCCCC, DefaultSelectionBackground = 0x784F26,
                CursorStyle = CursorStyle.BlinkingBar,
                ColorTable = [0x0C0C0C, 0x1F0FC5, 0x0EA113, 0x009CC1, 0xDA3700, 0x981788, 0xDD963A, 0xCCCCCC, 0x767676, 0x5648E7, 0x0CC616, 0xA5F1F9, 0xFF783B, 0x9E00B4, 0xD6D661, 0xF2F2F2]
            }, "Cascadia Mono", 13);
            control.GotFocus += async (_, _) =>
            {
                if (ActiveTab?.ActivePaneId == pane.Id || busy) return;
                await Run(() => Change("pane.focus", new { paneId = pane.Id }, false));
            };
            panel.Children.Add(control);
            return new Border { BorderBrush = ThemeBrush("TerminalBorderBrush"), BorderThickness = new Thickness(1), Child = panel };
        }
        var grid = new Grid();
        var vertical = node.Orientation == "vertical";
        if (vertical)
        {
            grid.ColumnDefinitions.Add(new() { Width = new GridLength(node.Ratio, GridUnitType.Star), MinWidth = 40 });
            grid.ColumnDefinitions.Add(new() { Width = new GridLength(5) });
            grid.ColumnDefinitions.Add(new() { Width = new GridLength(1 - node.Ratio, GridUnitType.Star), MinWidth = 40 });
        }
        else
        {
            grid.RowDefinitions.Add(new() { Height = new GridLength(node.Ratio, GridUnitType.Star), MinHeight = 30 });
            grid.RowDefinitions.Add(new() { Height = new GridLength(5) });
            grid.RowDefinitions.Add(new() { Height = new GridLength(1 - node.Ratio, GridUnitType.Star), MinHeight = 30 });
        }
        var first = BuildLayout(node.First!, [.. path, 0]);
        var second = BuildLayout(node.Second!, [.. path, 1]);
        var splitter = new GridSplitter { Background = ThemeBrush("BorderBrush"), HorizontalAlignment = HorizontalAlignment.Stretch, VerticalAlignment = VerticalAlignment.Stretch, ResizeDirection = vertical ? GridResizeDirection.Columns : GridResizeDirection.Rows, ResizeBehavior = GridResizeBehavior.PreviousAndNext, ToolTip = "Drag to resize terminal panes" };
        if (vertical) { Grid.SetColumn(splitter, 1); Grid.SetColumn(second, 2); }
        else { Grid.SetRow(splitter, 1); Grid.SetRow(second, 2); }
        splitter.DragCompleted += async (_, _) => await Run(async () =>
        {
            var total = vertical ? grid.ColumnDefinitions[0].ActualWidth + grid.ColumnDefinitions[2].ActualWidth : grid.RowDefinitions[0].ActualHeight + grid.RowDefinitions[2].ActualHeight;
            var size = vertical ? grid.ColumnDefinitions[0].ActualWidth : grid.RowDefinitions[0].ActualHeight;
            var ratio = Math.Clamp(size / Math.Max(1, total), .1, .9);
            if (vertical) { grid.ColumnDefinitions[0].Width = new GridLength(ratio, GridUnitType.Star); grid.ColumnDefinitions[2].Width = new GridLength(1 - ratio, GridUnitType.Star); }
            else { grid.RowDefinitions[0].Height = new GridLength(ratio, GridUnitType.Star); grid.RowDefinitions[2].Height = new GridLength(1 - ratio, GridUnitType.Star); }
            await Change("pane.resize", new { tabId = ActiveTab!.Id, path, ratio }, false);
        });
        grid.Children.Add(first); grid.Children.Add(second); grid.Children.Add(splitter);
        return grid;
    }
    private async Task FocusPane(string paneId)
    {
        await Change("pane.focus", new { paneId }, false);
        if (controls.TryGetValue(paneId, out var control)) control.Focus();
    }
    private UIElement BuildSettingsPanel()
    {
        var root = new Grid { Background = ThemeBrush("EditorBackgroundBrush") };
        var scroll = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled };
        var content = new StackPanel { Margin = new Thickness(38, 30, 38, 30), MaxWidth = 760 };
        content.Children.Add(new TextBlock { Text = "Settings", FontSize = 26, FontWeight = FontWeights.SemiBold, Foreground = ThemeBrush("ForegroundBrush") });
        content.Children.Add(new TextBlock { Text = "Configure Paneacea defaults", FontSize = 13, Foreground = ThemeBrush("MutedForegroundBrush"), Margin = new Thickness(0, 5, 0, 24) });
        content.Children.Add(new TextBlock { Text = "Terminal", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = ThemeBrush("ForegroundBrush"), Margin = new Thickness(0, 0, 0, 10) });

        var shellSection = new Border { Background = ThemeBrush("PanelBackgroundBrush"), BorderBrush = ThemeBrush("BorderBrush"), BorderThickness = new Thickness(1), Padding = new Thickness(16), Margin = new Thickness(0, 0, 0, 24) };
        var shellGrid = new Grid();
        shellGrid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        shellGrid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        shellGrid.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        shellGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        shellGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var shellLabel = new StackPanel();
        shellLabel.Children.Add(new TextBlock { Text = "Default shell", FontSize = 14, Foreground = ThemeBrush("ForegroundBrush") });
        shellLabel.Children.Add(new TextBlock { Text = "Used for new tabs and split panes", FontSize = 11, Foreground = ThemeBrush("MutedForegroundBrush"), Margin = new Thickness(0, 4, 0, 0) });
        Grid.SetRow(shellLabel, 0); Grid.SetColumn(shellLabel, 0); shellGrid.Children.Add(shellLabel);
        var combo = new ComboBox { Name = "DefaultShellComboBox", ItemsSource = shells.Where(shell => shell.IsAvailable).ToList(), DisplayMemberPath = "Name", MinWidth = 240, ToolTip = "Select the shell launched for new terminals" };
        combo.SelectedItem = DefaultShell;
        combo.SelectionChanged += async (_, _) =>
        {
            if (combo.SelectedItem is ShellProfile shell && shell.IsAvailable && !busy)
                await Run(() => SetDefaultShell(shell));
        };
        Grid.SetRow(combo, 0); Grid.SetColumn(combo, 1); shellGrid.Children.Add(combo);
        var selected = DefaultShell;
        var selectedPath = new TextBlock { Text = selected is null ? "No supported shell was detected." : $"{selected.Description} · {selected.Executable}", FontSize = 11, Foreground = ThemeBrush("MutedForegroundBrush"), TextWrapping = TextWrapping.Wrap, Margin = new Thickness(0, 16, 0, 0) };
        Grid.SetRow(selectedPath, 1); Grid.SetColumnSpan(selectedPath, 2); shellGrid.Children.Add(selectedPath);
        shellSection.Child = shellGrid; content.Children.Add(shellSection);

        content.Children.Add(new TextBlock { Text = "Detected shells", FontSize = 16, FontWeight = FontWeights.SemiBold, Foreground = ThemeBrush("ForegroundBrush"), Margin = new Thickness(0, 0, 0, 10) });
        var detected = new Border { Background = ThemeBrush("PanelBackgroundBrush"), BorderBrush = ThemeBrush("BorderBrush"), BorderThickness = new Thickness(1), Padding = new Thickness(16), Margin = new Thickness(0, 0, 0, 24) };
        var detectedPanel = new StackPanel();
        foreach (var shell in shells)
        {
            var row = new Grid { Margin = new Thickness(0, 0, 0, 12) };
            row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            row.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
            var details = new StackPanel();
            details.Children.Add(new TextBlock { Text = shell.Name, FontSize = 13, Foreground = ThemeBrush("ForegroundBrush") });
            details.Children.Add(new TextBlock { Text = shell.Executable, FontSize = 11, Foreground = ThemeBrush("MutedForegroundBrush"), Margin = new Thickness(0, 3, 0, 0), TextTrimming = TextTrimming.CharacterEllipsis });
            Grid.SetColumn(details, 0); row.Children.Add(details);
            var availability = new TextBlock { Text = shell.IsAvailable ? "Available" : "Not found", FontSize = 11, Foreground = ThemeBrush(shell.IsAvailable ? "SuccessBrush" : "MutedForegroundBrush"), VerticalAlignment = VerticalAlignment.Center };
            Grid.SetColumn(availability, 1); row.Children.Add(availability);
            detectedPanel.Children.Add(row);
        }
        detected.Child = detectedPanel; content.Children.Add(detected);
        var back = new Button { Name = "SettingsBackButton", Content = "Back to terminal", HorizontalAlignment = HorizontalAlignment.Left, ToolTip = "Return to the terminal workspace" };
        back.Click += (_, _) => { showingSettings = false; Render(); };
        content.Children.Add(back);
        scroll.Content = content; root.Children.Add(scroll);
        return root;
    }
    private Task SetDefaultShell(ShellProfile shell) => Change("settings.set", new { key = "defaultShell", value = ShellSetting(shell) });
    private object NewTabParameters(string workspaceId)
    {
        return DefaultShell is { } shell
            ? new { workspaceId, executable = shell.Executable, arguments = shell.Arguments }
            : new { workspaceId };
    }
    private object SplitParameters(string paneId, string orientation)
    {
        return DefaultShell is { } shell
            ? new { paneId, orientation, executable = shell.Executable, arguments = shell.Arguments }
            : new { paneId, orientation };
    }
    private string? Prompt(string title, string label, string value, bool browseForFolder = false)
    {
        dialog = true;
        try
        {
            var window = new Window { Owner = this, Title = title, Width = 480, Height = 170, ResizeMode = ResizeMode.NoResize, WindowStartupLocation = WindowStartupLocation.CenterOwner, Background = ThemeBrush("SidebarBackgroundBrush"), Foreground = ThemeBrush("ForegroundBrush") };
            var panel = new StackPanel { Margin = new Thickness(12) };
            var input = new TextBox { Text = value };
            var save = new Button { Content = "Save", IsDefault = true };
            save.Click += (_, _) => { if (!string.IsNullOrWhiteSpace(input.Text)) window.DialogResult = true; };
            panel.Children.Add(new TextBlock { Text = label });
            if (browseForFolder)
            {
                var inputRow = new Grid();
                inputRow.ColumnDefinitions.Add(new ColumnDefinition());
                inputRow.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
                var browse = new Button { Content = "Browse…", Name = "BrowseButton", ToolTip = "Browse for a folder in File Explorer" };
                browse.Click += (_, _) =>
                {
                    var folderDialog = new Microsoft.Win32.OpenFolderDialog
                    {
                        Multiselect = false,
                        Title = "Select workspace root"
                    };
                    if (Directory.Exists(input.Text)) folderDialog.InitialDirectory = input.Text;
                    if (folderDialog.ShowDialog(window) == true)
                    {
                        input.Text = folderDialog.FolderName;
                        input.CaretIndex = input.Text.Length;
                    }
                };
                Grid.SetColumn(input, 0); Grid.SetColumn(browse, 1);
                inputRow.Children.Add(input); inputRow.Children.Add(browse);
                panel.Children.Add(inputRow);
            }
            else panel.Children.Add(input);
            panel.Children.Add(save);
            window.Content = panel; window.Loaded += (_, _) => { input.Focus(); input.SelectAll(); };
            return window.ShowDialog() == true ? input.Text.Trim() : null;
        }
        finally { dialog = false; }
    }
    private async Task NewWorkspace()
    {
        var name = Prompt("New workspace", "Name", "Workspace"); if (name is null) return;
        var root = Prompt("Workspace root", "Existing absolute directory", ActiveWorkspace?.RootDirectory ?? Environment.CurrentDirectory, true); if (root is null) return;
        await Change("workspace.create", new { name, rootDirectory = root }, false);
        await Change("tab.create", NewTabParameters(state.ActiveWorkspaceId!));
    }
    private Task NewTab()
    {
        showingSettings = false;
        return ActiveWorkspace is { } workspace ? Change("tab.create", NewTabParameters(workspace.Id)) : Task.CompletedTask;
    }
    private Task Split(string orientation)
    {
        showingSettings = false;
        return ActiveTab is { } tab ? Change("pane.split", SplitParameters(tab.ActivePaneId, orientation)) : Task.CompletedTask;
    }
    private async Task Execute(string action)
    {
        switch (action)
        {
            case "Terminal.NewTab": await NewTab(); break;
            case "Terminal.SplitPaneRight": await Split("vertical"); break;
            case "Terminal.SplitPaneDown": await Split("horizontal"); break;
            case "Terminal.SplitPaneAuto": await Split(PaneHost.ActualWidth >= PaneHost.ActualHeight ? "vertical" : "horizontal"); break;
            case "Preferences.Settings": showingSettings = true; Render(); break;
            case "Terminal.ClosePane": if (ActiveTab is { } close) await Change("pane.close", new { paneId = close.ActivePaneId }); break;
            case "Terminal.OpenTabRenamer": if (ActiveTab is { } tab && Prompt("Rename tab", "Title", tab.Title) is { } title) await Change("tab.rename", new { tabId = tab.Id, title }); break;
            case "Workspace.New": await NewWorkspace(); break;
            case "Workspace.Close": if (ActiveWorkspace is { } closeWorkspace) await Change("workspace.close", new { workspaceId = closeWorkspace.Id }); break;
            case "Workspace.Rename": if (ActiveWorkspace is { } rename && Prompt("Rename workspace", "Name", rename.Name) is { } name) await Change("workspace.rename", new { workspaceId = rename.Id, name }); break;
            case "Workspace.ChangeRoot": if (ActiveWorkspace is { } root && Prompt("Workspace root", "Existing absolute directory", root.RootDirectory, true) is { } directory) await Change("workspace.setRoot", new { workspaceId = root.Id, rootDirectory = directory }); break;
            case "Workspace.OpenRoot": if (ActiveWorkspace is { } open) Process.Start(new ProcessStartInfo(open.RootDirectory) { UseShellExecute = true }); break;
            case "Workspace.Next": case "Workspace.Previous":
                if (state.Workspaces.Count > 0) { var index = state.Workspaces.FindIndex(w => w.Id == state.ActiveWorkspaceId); index = (index + state.Workspaces.Count + (action == "Workspace.Next" ? 1 : -1)) % state.Workspaces.Count; await Change("workspace.switch", new { workspaceId = state.Workspaces[index].Id }); } break;
            case "Terminal.Reconnect": state = await client.State(); Render(); break;
            default:
                if (ActiveTab is not { } active) break;
                if (action.StartsWith("Terminal.MoveFocus") && LayoutGeometry.Neighbor(active.RootLayoutNode, active.ActivePaneId, action[18..]) is { } neighbor) await FocusPane(neighbor);
                if (action.StartsWith("Terminal.ResizePane") && LayoutGeometry.Resize(active.RootLayoutNode, active.ActivePaneId, action[19..]) is { } resize) await Change("pane.resize", new { tabId = active.Id, path = resize.Path, ratio = resize.Ratio });
                break;
        }
    }
    private void KeyMessage(ref MSG message, ref bool handled)
    {
        if (handled || !IsActive || dialog || message.message is not (0x100 or 0x104)) return;
        var key = KeyInterop.KeyFromVirtualKey((int)message.wParam);
        var modifiers = Keyboard.Modifiers;
        string? action = null;
        if (modifiers == (ModifierKeys.Control | ModifierKeys.Shift)) action = key switch { Key.T => "Terminal.NewTab", Key.W => "Terminal.ClosePane", Key.P => "Palette", _ => null };
        if (modifiers == ModifierKeys.Alt && key is Key.Left or Key.Right or Key.Up or Key.Down) action = "Terminal.MoveFocus" + key;
        if (modifiers == (ModifierKeys.Alt | ModifierKeys.Shift)) action = key switch
        {
            Key.D => "Terminal.SplitPaneAuto", Key.OemMinus => "Terminal.SplitPaneDown", Key.OemPlus => "Terminal.SplitPaneRight",
            Key.Left or Key.Right or Key.Up or Key.Down => "Terminal.ResizePane" + key, _ => null
        };
        if (action is null) return;
        handled = true;
        var selected = action;
        Dispatcher.BeginInvoke(async () => { if (selected == "Palette") Commands(); else await Run(() => Execute(selected)); });
    }
    private void Commands()
    {
        dialog = true;
        string? selected = null;
        try
        {
            var commandMap = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                ["Terminal: New Tab"] = "Terminal.NewTab",
                ["Terminal: Split Right"] = "Terminal.SplitPaneRight",
                ["Terminal: Split Down"] = "Terminal.SplitPaneDown",
                ["Terminal: Close Pane"] = "Terminal.ClosePane",
                ["Terminal: Rename Tab"] = "Terminal.OpenTabRenamer",
                ["Preferences: Open Settings"] = "Preferences.Settings",
                ["Workspace: Open Folder"] = "Workspace.New",
                ["Workspace: Rename Workspace"] = "Workspace.Rename",
                ["Workspace: Change Workspace Folder"] = "Workspace.ChangeRoot",
                ["Workspace: Open Folder in Explorer"] = "Workspace.OpenRoot",
                ["Workspace: Next"] = "Workspace.Next",
                ["Workspace: Previous"] = "Workspace.Previous",
                ["Workspace: Close"] = "Workspace.Close",
                ["Paneacea: Reconnect Runtime"] = "Terminal.Reconnect"
            };
            string[] actions = [.. commandMap.Keys];
            var window = new Window { Owner = this, Title = "Command Palette", Width = 460, Height = 450, WindowStartupLocation = WindowStartupLocation.CenterOwner, Background = ThemeBrush("SidebarBackgroundBrush"), Foreground = ThemeBrush("ForegroundBrush") };
            var panel = new DockPanel { Margin = new Thickness(12) };
            var filter = new TextBox(); DockPanel.SetDock(filter, Dock.Top);
            var list = new ListBox { ItemsSource = actions, SelectedIndex = 0 };
            filter.TextChanged += (_, _) => { list.ItemsSource = actions.Where(a => a.Contains(filter.Text, StringComparison.OrdinalIgnoreCase)).ToArray(); list.SelectedIndex = 0; };
            void Choose() { if (list.SelectedItem is string action && commandMap.TryGetValue(action, out var command)) { selected = command; window.DialogResult = true; } }
            list.MouseDoubleClick += (_, _) => Choose();
            window.PreviewKeyDown += (_, e) => { if (e.Key == Key.Enter) { Choose(); e.Handled = true; } if (e.Key == Key.Escape) window.Close(); };
            panel.Children.Add(filter); panel.Children.Add(list); window.Content = panel;
            window.Loaded += (_, _) => filter.Focus(); window.ShowDialog();
        }
        finally { dialog = false; }
        if (selected is not null) _ = Run(() => Execute(selected));
    }
    private async void WorkspaceList_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (!updating && WorkspaceList.SelectedItem is Workspace workspace && workspace.Id != state.ActiveWorkspaceId)
            await Run(() => Change("workspace.switch", new { workspaceId = workspace.Id }));
    }
    private async void NewTab_Click(object sender, RoutedEventArgs e) => await Run(NewTab);
    private async void NewWorkspace_Click(object sender, RoutedEventArgs e) => await Run(NewWorkspace);
    private async void SplitRight_Click(object sender, RoutedEventArgs e) => await Run(() => Split("vertical"));
    private async void SplitDown_Click(object sender, RoutedEventArgs e) => await Run(() => Split("horizontal"));
    private void Commands_Click(object sender, RoutedEventArgs e) => Commands();
    private void Settings_Click(object sender, RoutedEventArgs e) { showingSettings = true; Render(); }
    private void TitleBar_MouseLeftButtonDown(object sender, MouseButtonEventArgs e)
    {
        if (e.ChangedButton != MouseButton.Left || FindVisualParent<Button>(e.OriginalSource as DependencyObject) is not null) return;
        e.Handled = true;
        if (e.ClickCount == 2) MaximizeWindow_Click(sender, e);
        else DragMove();
    }
    private void MinimizeWindow_Click(object sender, RoutedEventArgs e) => WindowState = WindowState.Minimized;
    private void MaximizeWindow_Click(object sender, RoutedEventArgs e) => WindowState = WindowState == WindowState.Maximized ? WindowState.Normal : WindowState.Maximized;
    private void CloseWindow_Click(object sender, RoutedEventArgs e) => Close();
    private static T? FindVisualParent<T>(DependencyObject? element) where T : DependencyObject
    {
        while (element is not null)
        {
            if (element is T parent) return parent;
            element = VisualTreeHelper.GetParent(element);
        }
        return null;
    }
}
