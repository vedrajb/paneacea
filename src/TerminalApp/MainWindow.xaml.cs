using Microsoft.Terminal.Wpf;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Data;
using System.Windows.Input;
using System.Windows.Interop;
using System.Windows.Media;
using System.Windows.Threading;
using Paneacea.Core;

namespace Paneacea;

public partial class MainWindow : Window
{
    private const int WmGetMinMaxInfo = 0x0024;
    private const uint MonitorDefaultToNearest = 0x00000002;
    private const int DefaultTerminalFontSize = 11;
    private const int MinimumTerminalFontSize = 8;
    private const int MaximumTerminalFontSize = 32;
    private const int DefaultTerminalHistoryLines = 2000;
    private static readonly int[] TerminalHistoryChoices = [0, 500, 2000, 5000, 10000, 25000];
    private readonly RuntimeClient client = new(Environment.GetEnvironmentVariable("PANEACEA_PIPE"));
    private WorkspaceState state = new();
    private readonly Dictionary<string, TerminalControl> controls = [];
    private readonly Dictionary<string, Border> paneBorders = [];
    private readonly Dictionary<string, Button> paneHeaders = [];
    private readonly List<PipeTerminalConnection> connections = [];
    private readonly HashSet<int> pressedModifierKeys = [];
    private readonly DispatcherTimer terminalStatusTimer = new() { Interval = TimeSpan.FromMilliseconds(500) };
    private bool updating;
    private bool busy;
    private bool refreshingTerminalStatus;
    private bool dialog;
    private bool showingSettings;
    private IReadOnlyList<ShellProfile> shells = [];
    private HwndSource? windowSource;
    private Workspace? ActiveWorkspace => state.Workspaces.FirstOrDefault(w => w.Id == state.ActiveWorkspaceId);
    private Tab? ActiveTab => ActiveWorkspace?.Tabs.FirstOrDefault(t => t.Id == ActiveWorkspace.ActiveTabId);
    private int TerminalFontSize
    {
        get
        {
            if (!state.Settings.TryGetValue("terminalFontSize", out var setting)
                || setting.ValueKind != JsonValueKind.Number
                || !setting.TryGetInt32(out var size)) return DefaultTerminalFontSize;
            return Math.Clamp(size, MinimumTerminalFontSize, MaximumTerminalFontSize);
        }
    }
    private int TerminalHistoryLines
    {
        get
        {
            if (!state.Settings.TryGetValue("terminalHistoryLines", out var setting)
                || setting.ValueKind != JsonValueKind.Number
                || !setting.TryGetInt32(out var lines)
                || !TerminalHistoryChoices.Contains(lines)) return DefaultTerminalHistoryLines;
            return lines;
        }
    }
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
    private sealed record CommandPaletteItem(string Label, string Shortcut);
    private sealed record HistoryChoice(int Lines, string Label);

    public MainWindow()
    {
        InitializeComponent();
        AppLogger.Initialize();
        AppLogger.Info(
            "app.start",
            $"pipe={client.PipeName} base_directory={AppContext.BaseDirectory} data={Environment.GetEnvironmentVariable("PANEACEA_DATA") ?? "default"} log={AppLogger.LogPath}");
        SourceInitialized += (_, _) =>
        {
            windowSource = (HwndSource)PresentationSource.FromVisual(this)!;
            windowSource.AddHook(WindowMessageFilter);
        };
        Loaded += async (_, _) =>
        {
            await Run(Initialize);
            terminalStatusTimer.Start();
            _ = Dispatcher.BeginInvoke(FocusActiveTerminal, DispatcherPriority.ApplicationIdle);
        };
        terminalStatusTimer.Tick += async (_, _) =>
        {
            await SyncPaneFocus();
            await RefreshTerminalStatus();
        };
        ComponentDispatcher.ThreadPreprocessMessage += KeyMessage;
        ComponentDispatcher.ThreadFilterMessage += TerminalNavigationMessage;
        Activated += (_, _) =>
        {
            if (!dialog && !showingSettings)
                _ = Dispatcher.BeginInvoke(FocusActiveTerminal, DispatcherPriority.Input);
        };
        Deactivated += (_, _) => pressedModifierKeys.Clear();
        StateChanged += (_, _) =>
        {
            MaximizeWindowButton.Content = WindowState == WindowState.Maximized ? "\uE923" : "\uE922";
            MaximizeWindowButton.ToolTip = WindowState == WindowState.Maximized ? "Restore" : "Maximize";
        };
        Closed += (_, _) =>
        {
            AppLogger.Info("app.closed", "window closed");
            terminalStatusTimer.Stop(); ComponentDispatcher.ThreadPreprocessMessage -= KeyMessage; windowSource?.RemoveHook(WindowMessageFilter); Detach();
            ComponentDispatcher.ThreadFilterMessage -= TerminalNavigationMessage;
        };
    }
    private static IntPtr WindowMessageFilter(IntPtr hwnd, int message, IntPtr wParam, IntPtr lParam, ref bool handled)
    {
        if (message != WmGetMinMaxInfo) return IntPtr.Zero;
        var maxInfo = Marshal.PtrToStructure<NativeMinMaxInfo>(lParam);
        var monitor = MonitorFromWindow(hwnd, MonitorDefaultToNearest);
        if (monitor == IntPtr.Zero) return IntPtr.Zero;
        var monitorInfo = new NativeMonitorInfo { Size = Marshal.SizeOf<NativeMonitorInfo>() };
        if (!GetMonitorInfo(monitor, ref monitorInfo)) return IntPtr.Zero;
        maxInfo.MaxPosition.X = monitorInfo.Work.Left - monitorInfo.Monitor.Left;
        maxInfo.MaxPosition.Y = monitorInfo.Work.Top - monitorInfo.Monitor.Top;
        maxInfo.MaxSize.X = monitorInfo.Work.Right - monitorInfo.Work.Left;
        maxInfo.MaxSize.Y = monitorInfo.Work.Bottom - monitorInfo.Work.Top;
        Marshal.StructureToPtr(maxInfo, lParam, false);
        handled = true;
        return IntPtr.Zero;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct NativePoint
    {
        public int X;
        public int Y;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct NativeRect
    {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct NativeMinMaxInfo
    {
        public NativePoint Reserved;
        public NativePoint MaxSize;
        public NativePoint MaxPosition;
        public NativePoint MinTrackSize;
        public NativePoint MaxTrackSize;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct NativeMonitorInfo
    {
        public int Size;
        public NativeRect Monitor;
        public NativeRect Work;
        public uint Flags;
    }
    [DllImport("user32.dll")]
    private static extern IntPtr GetFocus();
    [DllImport("user32.dll", EntryPoint = "DispatchMessageW")]
    private static extern IntPtr DispatchMessage(ref MSG message);
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool IsChild(IntPtr parent, IntPtr child);
    [DllImport("user32.dll")]
    private static extern IntPtr MonitorFromWindow(IntPtr hwnd, uint flags);
    [DllImport("user32.dll")]
    private static extern IntPtr SetFocus(IntPtr hwnd);
    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetMonitorInfo(IntPtr monitor, ref NativeMonitorInfo monitorInfo);
    private async Task Run(Func<Task> action)
    {
        if (busy) return;
        busy = true;
        try { await action(); }
        catch (Exception error)
        {
            AppLogger.Error("app.action.failed", $"error={error.Message}");
            Report(error.Message);
        }
        finally { busy = false; }
    }
    private void Report(string message)
    {
        AppLogger.Error("app.status", $"message={message}");
        Dispatcher.BeginInvoke(() => Status.Text = message);
    }
    private static void LogState(string eventName, WorkspaceState value)
    {
        var paneDetails = string.Join(
            ";",
            value.Panes.Values
                .OrderBy(pane => pane.Id)
                .Select(pane => $"{pane.Id}:cwd={pane.CurrentWorkingDirectory}:exe={Path.GetFileName(pane.Executable)}"));
        AppLogger.Info(
            eventName,
            $"workspaces={value.Workspaces.Count} tabs={value.Workspaces.Sum(workspace => workspace.Tabs.Count)} panes={value.Panes.Count} settings={value.Settings.Count} active_workspace={value.ActiveWorkspaceId ?? "none"} pane_details={paneDetails}");
    }
    private async Task Initialize()
    {
        AppLogger.Info("app.initialize.start", $"pipe={client.PipeName}");
        try
        {
            state = await client.State();
            LogState("app.state.loaded", state);
        }
        catch (Exception error) when (error is TimeoutException or IOException or OperationCanceledException)
        {
            AppLogger.Info("app.runtime.start", $"reason={error.Message}");
            var executable = Path.Combine(AppContext.BaseDirectory, "panacea-runtime.exe");
            if (!File.Exists(executable)) throw new FileNotFoundException("Build with scripts/build.ps1 so panacea-runtime.exe is beside Paneacea.App.exe.");
            var start = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = AppContext.BaseDirectory };
            start.ArgumentList.Add("--pipe"); start.ArgumentList.Add(client.PipeName);
            if (Environment.GetEnvironmentVariable("PANEACEA_DATA") is { Length: > 0 } directory) { start.ArgumentList.Add("--data"); start.ArgumentList.Add(directory); }
            using var process = Process.Start(start);
            AppLogger.Info("app.runtime.started", $"executable={executable} data={Environment.GetEnvironmentVariable("PANEACEA_DATA") ?? "default"}");
            for (var attempt = 0; ; attempt++)
            {
                try
                {
                    state = await client.State();
                    LogState("app.state.loaded_after_runtime_start", state);
                    break;
                }
                catch (Exception retryError) when (attempt < 4)
                {
                    AppLogger.Error("app.runtime.wait", $"attempt={attempt + 1} error={retryError.Message}");
                    await Task.Delay(250);
                }
            }
        }
        await EnsureDefaultShell();
        if (state.Workspaces.Count == 0)
        {
            await Change("workspace.create", new { name = "Default", rootDirectory = Environment.CurrentDirectory }, false);
            await Change("tab.create", NewTabParameters(state.ActiveWorkspaceId!), false);
        }
        LogState("app.initialize.complete", state);
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
        AppLogger.Info("app.change.start", $"method={method}");
        state = (await client.Call(method, parameters)).Deserialize<WorkspaceState>(RuntimeClient.JsonOptions)!;
        LogState("app.change.complete", state);
        if (render) Render();
    }
    private void Detach()
    {
        AppLogger.Info("app.terminals.detach", $"connections={connections.Count} controls={controls.Count}");
        foreach (var connection in connections) connection.Dispose();
        connections.Clear(); controls.Clear(); paneBorders.Clear(); paneHeaders.Clear(); PaneHost.Content = null;
    }
    private void Render()
    {
        AppLogger.Info(
            "app.render",
            $"active_workspace={state.ActiveWorkspaceId ?? "none"} active_tab={ActiveTab?.Id ?? "none"} active_pane={ActiveTab?.ActivePaneId ?? "none"} panes={state.Panes.Count}");
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
                BorderThickness = isActiveTab ? new Thickness(0, 0, 0, 3) : new Thickness(0, 0, 0, 1),
                MinHeight = 40,
                Padding = new Thickness(14, 0, 14, 0),
                ToolTip = $"Terminal tab: {tab.Title}. Right-click to rename or close."
            };
            button.Click += async (_, _) => await Run(() => Change("tab.focus", new { tabId = tab.Id }));
            var menu = new ContextMenu();
            var rename = new MenuItem { Header = "Rename tab" };
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
            _ = Dispatcher.BeginInvoke(FocusActiveTerminal, DispatcherPriority.ApplicationIdle);
        }
        else PaneHost.Content = new TextBlock { Text = "Open a workspace folder or create a new terminal tab to start.", Margin = new Thickness(30), Foreground = ThemeBrush("MutedForegroundBrush") };
        Status.Text = ActiveWorkspace is { } workspace ? $"{workspace.Name}  ·  {workspace.RootDirectory}  ·  {DefaultShell?.Name ?? "No shell"}" : "No workspace open";
        RuntimeState.Text = "Runtime · Connected";
    }
    private async Task RefreshTerminalStatus()
    {
        if (busy || refreshingTerminalStatus || showingSettings || state.Workspaces.Count == 0) return;
        refreshingTerminalStatus = true;
        try
        {
            var refreshed = await client.State();
            var previousWorkspace = ActiveWorkspace;
            var refreshedWorkspace = refreshed.Workspaces.FirstOrDefault(workspace => workspace.Id == refreshed.ActiveWorkspaceId);
            if (previousWorkspace?.Id != refreshedWorkspace?.Id
                || previousWorkspace is not null && refreshedWorkspace is null
                || previousWorkspace is null && refreshedWorkspace is not null
                || previousWorkspace is not null && refreshedWorkspace is not null
                    && !previousWorkspace.Tabs.Select(tab => tab.Id).SequenceEqual(refreshedWorkspace.Tabs.Select(tab => tab.Id)))
            {
                state = refreshed;
                Render();
                return;
            }
            state = refreshed;
            UpdateTerminalStatusVisuals();
        }
        catch (Exception error) when (error is TimeoutException or IOException or OperationCanceledException)
        {
        }
        finally
        {
            refreshingTerminalStatus = false;
        }
    }
    private void UpdateTerminalStatusVisuals()
    {
        foreach (var paneHeader in paneHeaders)
        {
            if (state.Panes.TryGetValue(paneHeader.Key, out var pane))
            {
                paneHeader.Value.Content = TerminalHeader(pane);
                paneHeader.Value.ToolTip = TerminalToolTip(pane);
            }
        }
    }
    private static string TerminalHeader(Pane pane)
    {
        var title = string.IsNullOrWhiteSpace(pane.Title) ? Path.GetFileName(pane.Executable) : pane.Title;
        var commandOrApp = IsShellTitle(pane, title) ? Path.GetFileName(pane.Executable) : title;
        return $"{FolderName(pane.CurrentWorkingDirectory)} | {commandOrApp}";
    }
    private static string FolderName(string directory)
    {
        var trimmed = directory.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
        var name = Path.GetFileName(trimmed);
        return string.IsNullOrWhiteSpace(name) ? directory : name;
    }
    private static bool IsShellTitle(Pane pane, string title)
    {
        var executable = Path.GetFileName(pane.Executable);
        var executableName = Path.GetFileNameWithoutExtension(executable);
        return title.Equals("PowerShell", StringComparison.OrdinalIgnoreCase)
            || title.Equals("Git Bash", StringComparison.OrdinalIgnoreCase)
            || title.Equals("Command Prompt", StringComparison.OrdinalIgnoreCase)
            || title.Equals(executable, StringComparison.OrdinalIgnoreCase)
            || title.Equals(pane.Executable, StringComparison.OrdinalIgnoreCase)
            || !string.IsNullOrWhiteSpace(executableName) && title.Contains(executableName, StringComparison.OrdinalIgnoreCase);
    }
    private static string TerminalToolTip(Pane pane)
    {
        var command = string.Join(" ", new[] { Path.GetFileName(pane.Executable) }.Concat(pane.Arguments));
        var title = string.IsNullOrWhiteSpace(pane.Title) ? Path.GetFileName(pane.Executable) : pane.Title;
        return $"{title}\n{command}\n{pane.CurrentWorkingDirectory}";
    }
    private UIElement BuildLayout(LayoutNode node, int[] path)
    {
        if (node.Type == "pane")
        {
            var pane = state.Panes[node.PaneId!];
            var panel = new DockPanel();
            var header = new Button
            {
                Content = TerminalHeader(pane),
                HorizontalContentAlignment = HorizontalAlignment.Left,
                Padding = new Thickness(10, 3, 10, 3),
                Margin = new Thickness(0),
                Background = ThemeBrush("PanelBackgroundBrush"),
                Foreground = ThemeBrush("MutedForegroundBrush"),
                BorderBrush = ThemeBrush("BorderBrush"),
                BorderThickness = new Thickness(0, 0, 0, 1),
                ToolTip = TerminalToolTip(pane)
            };
            DockPanel.SetDock(header, Dock.Top); panel.Children.Add(header);
            paneHeaders[pane.Id] = header;
            header.Click += async (_, _) => await Run(() => FocusPane(pane.Id));
            if (pane.Error is not null) { panel.Children.Add(new TextBlock { Text = pane.Error, TextWrapping = TextWrapping.Wrap, Margin = new Thickness(15), Foreground = ThemeBrush("ErrorBrush") }); return panel; }
            var control = new TerminalControl { Focusable = true };
            var connection = new PipeTerminalConnection(client, pane.Id, Report);
            control.Connection = connection;
            controls[pane.Id] = control; connections.Add(connection);
            var isActivePane = ActiveTab?.ActivePaneId == pane.Id;
            var border = new Border
            {
                BorderBrush = ThemeBrush(isActivePane ? "FocusBrush" : "TerminalBorderBrush"),
                BorderThickness = new Thickness(isActivePane ? 2 : 1),
                Child = panel
            };
            paneBorders[pane.Id] = border;
            control.Loaded += (_, _) =>
            {
                ApplyTerminalTheme(control, TerminalFontSize);
                if (ActiveTab?.ActivePaneId == pane.Id)
                {
                    SetPaneFocusVisual(pane.Id);
                    FocusTerminal(control);
                }
            };
            control.Loaded += (_, _) => ConfigureScrollBar(control, connection);
            control.PreviewMouseDown += async (_, _) =>
            {
                SetPaneFocusVisual(pane.Id);
                FocusTerminal(control);
                if (ActiveTab?.ActivePaneId == pane.Id || busy) return;
                await Run(() => FocusPane(pane.Id));
            };
            control.PreviewMouseWheel += async (_, e) =>
            {
                if ((Keyboard.Modifiers & ModifierKeys.Control) == 0) return;
                e.Handled = true;
                await Run(() => AdjustTerminalFontSize(e.Delta > 0 ? 1 : -1));
            };
            control.IsKeyboardFocusWithinChanged += async (_, _) =>
            {
                if (!control.IsKeyboardFocusWithin) return;
                SetPaneFocusVisual(pane.Id);
                if (ActiveTab?.ActivePaneId == pane.Id || busy) return;
                await Run(() => Change("pane.focus", new { paneId = pane.Id }, false));
            };
            control.LostFocus += (_, _) =>
            {
                if (ActiveTab?.ActivePaneId != pane.Id)
                {
                    border.BorderBrush = ThemeBrush("TerminalBorderBrush");
                    border.BorderThickness = new Thickness(1);
                }
            };
            panel.Children.Add(control);
            return border;
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
        var splitter = new GridSplitter { Focusable = false, IsTabStop = false, Background = ThemeBrush("BorderBrush"), HorizontalAlignment = HorizontalAlignment.Stretch, VerticalAlignment = VerticalAlignment.Stretch, ResizeDirection = vertical ? GridResizeDirection.Columns : GridResizeDirection.Rows, ResizeBehavior = GridResizeBehavior.PreviousAndNext, ToolTip = "Drag to resize terminal panes" };
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
    private void ApplyTerminalTheme(TerminalControl control, int fontSize)
    {
        control.SetTheme(new TerminalTheme
        {
            DefaultBackground = 0x1E1E1E, DefaultForeground = 0xCCCCCC, DefaultSelectionBackground = 0x784F26,
            CursorStyle = CursorStyle.BlinkingBar,
            ColorTable = [0x0C0C0C, 0x1F0FC5, 0x0EA113, 0x009CC1, 0xDA3700, 0x981788, 0xDD963A, 0xCCCCCC, 0x767676, 0x5648E7, 0x0CC616, 0xA5F1F9, 0xFF783B, 0x9E00B4, 0xD6D661, 0xF2F2F2]
        }, "Cascadia Mono", (short)fontSize);
    }
    private void ApplyTerminalFontSize(int fontSize)
    {
        foreach (var control in controls.Values)
            if (control.IsLoaded) ApplyTerminalTheme(control, fontSize);
    }
    private Task AdjustTerminalFontSize(int delta) => SetTerminalFontSize(TerminalFontSize + delta);
    private async Task SetTerminalFontSize(int fontSize)
    {
        fontSize = Math.Clamp(fontSize, MinimumTerminalFontSize, MaximumTerminalFontSize);
        if (fontSize == TerminalFontSize) return;
        await Change("settings.set", new { key = "terminalFontSize", value = fontSize }, false);
        ApplyTerminalFontSize(fontSize);
    }
    private void ConfigureScrollBar(TerminalControl control, PipeTerminalConnection connection)
    {
        var scrollbar = FindVisualChild<ScrollBar>(control);
        if (scrollbar is null || scrollbar.Orientation != Orientation.Vertical)
        {
            AppLogger.Error("app.scrollbar.missing", $"pane_id={connection.PaneId}");
            return;
        }
        AppLogger.Info("app.scrollbar.configure", $"pane_id={connection.PaneId} restored_viewport={connection.RestoredViewport?.ToString() ?? "none"}");
        const double indicatorWidth = 4;
        const double interactiveWidth = 16;
        scrollbar.Width = indicatorWidth;
        scrollbar.Background = Brushes.Transparent;
        scrollbar.Foreground = ThemeBrush("ScrollThumbBrush");
        scrollbar.BorderBrush = Brushes.Transparent;
        scrollbar.ApplyTemplate();
        var applyingViewport = false;
        var pendingViewport = 0UL;
        var viewportTimer = new DispatcherTimer { Interval = TimeSpan.FromMilliseconds(100) };
        viewportTimer.Tick += (_, _) =>
        {
            viewportTimer.Stop();
            connection.SetViewport(pendingViewport);
        };
        scrollbar.ValueChanged += (_, _) =>
        {
            if (applyingViewport) return;
            var maximum = Math.Max(0, scrollbar.Maximum - scrollbar.ViewportSize);
            pendingViewport = (ulong)Math.Max(0, Math.Round(maximum - scrollbar.Value));
            viewportTimer.Stop();
            viewportTimer.Start();
        };
        void SetHoverState(bool hovered)
        {
            scrollbar.Width = hovered ? interactiveWidth : indicatorWidth;
            if (FindVisualChild<Thumb>(scrollbar) is { } thumb)
            {
                thumb.Background = ThemeBrush(hovered ? "ScrollThumbHoverBrush" : "ScrollThumbBrush");
                thumb.BorderBrush = Brushes.Transparent;
                thumb.Cursor = Cursors.Hand;
                thumb.Opacity = hovered ? 1 : 0.65;
            }
            foreach (var button in FindVisualChildren<RepeatButton>(scrollbar))
                button.Visibility = Visibility.Collapsed;
        }
        scrollbar.MouseEnter += (_, _) => SetHoverState(true);
        scrollbar.MouseLeave += (_, _) => SetHoverState(false);
        SetHoverState(false);

        void ApplyViewport(ulong offset, int attempt = 0)
        {
            var maximum = Math.Max(0, scrollbar.Maximum - scrollbar.ViewportSize);
            if (offset > 0 && maximum == 0 && attempt < 60)
            {
                if (attempt == 0)
                    AppLogger.Info("app.scrollbar.restore.waiting", $"pane_id={connection.PaneId} offset={offset}");
                Dispatcher.BeginInvoke(() => ApplyViewport(offset, attempt + 1), DispatcherPriority.ContextIdle);
                return;
            }
            applyingViewport = true;
            scrollbar.Value = Math.Max(0, maximum - Math.Min((double)offset, maximum));
            applyingViewport = false;
            AppLogger.Info("app.scrollbar.restore.applied", $"pane_id={connection.PaneId} offset={offset} maximum={maximum} attempt={attempt}");
        }
        connection.ViewportRestored += offset =>
        {
            AppLogger.Info("app.scrollbar.restore.event", $"pane_id={connection.PaneId} offset={offset}");
            Dispatcher.BeginInvoke(() => ApplyViewport(offset), DispatcherPriority.Loaded);
        };
        if (connection.RestoredViewport is { } restoredViewport)
            Dispatcher.BeginInvoke(() => ApplyViewport(restoredViewport), DispatcherPriority.Loaded);
    }
    private static T? FindVisualChild<T>(DependencyObject root) where T : DependencyObject
    {
        if (root is T match) return match;
        for (var index = 0; index < VisualTreeHelper.GetChildrenCount(root); index++)
            if (FindVisualChild<T>(VisualTreeHelper.GetChild(root, index)) is { } child) return child;
        return null;
    }
    private static IEnumerable<T> FindVisualChildren<T>(DependencyObject root) where T : DependencyObject
    {
        if (root is T match) yield return match;
        for (var index = 0; index < VisualTreeHelper.GetChildrenCount(root); index++)
            foreach (var child in FindVisualChildren<T>(VisualTreeHelper.GetChild(root, index))) yield return child;
    }
    private void FocusActiveTerminal()
    {
        if (dialog || showingSettings || ActiveTab is not { } active || !controls.TryGetValue(active.ActivePaneId, out var control))
        {
            AppLogger.Info("app.focus.startup.skipped", $"dialog={dialog} settings={showingSettings} active_tab={ActiveTab?.Id ?? "none"} active_pane={ActiveTab?.ActivePaneId ?? "none"}");
            return;
        }
        AppLogger.Info("app.focus.startup", $"pane_id={active.ActivePaneId}");
        SetPaneFocusVisual(active.ActivePaneId);
        FocusTerminal(control);
    }
    private async Task SyncPaneFocus()
    {
        if (showingSettings || busy) return;
        foreach (var control in controls)
            if (HasTerminalFocus(control.Value) || control.Value.IsKeyboardFocusWithin)
            {
                SetPaneFocusVisual(control.Key);
                if (ActiveTab?.ActivePaneId != control.Key)
                    await Run(() => Change("pane.focus", new { paneId = control.Key }, false));
                return;
            }
    }
    private static bool HasTerminalFocus(TerminalControl control)
    {
        var container = FindVisualChild<TerminalContainer>(control);
        var focused = GetFocus();
        return container is not null && focused != IntPtr.Zero && (focused == container.Handle || IsChild(container.Handle, focused));
    }
    private static void FocusTerminal(TerminalControl control)
    {
        if (FindVisualChild<TerminalContainer>(control) is { } container)
        {
            container.Focus();
            Keyboard.Focus(container);
            if (container.Handle != IntPtr.Zero) SetFocus(container.Handle);
            return;
        }
        control.Focus();
        Keyboard.Focus(control);
    }
    private void SetPaneFocusVisual(string paneId)
    {
        foreach (var paneBorder in paneBorders)
        {
            var focused = paneBorder.Key == paneId;
            paneBorder.Value.BorderBrush = ThemeBrush(focused ? "FocusBrush" : "TerminalBorderBrush");
            paneBorder.Value.BorderThickness = new Thickness(focused ? 2 : 1);
        }
    }
    private async Task FocusPane(string paneId)
    {
        await Change("pane.focus", new { paneId }, false);
        SetPaneFocusVisual(paneId);
        if (controls.TryGetValue(paneId, out var control))
            FocusTerminal(control);
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

        var fontSection = new Border { Background = ThemeBrush("PanelBackgroundBrush"), BorderBrush = ThemeBrush("BorderBrush"), BorderThickness = new Thickness(1), Padding = new Thickness(16), Margin = new Thickness(0, 0, 0, 24) };
        var fontGrid = new Grid();
        fontGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        fontGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var fontLabel = new StackPanel();
        fontLabel.Children.Add(new TextBlock { Text = "Terminal font size", FontSize = 14, Foreground = ThemeBrush("ForegroundBrush") });
        fontLabel.Children.Add(new TextBlock { Text = $"{MinimumTerminalFontSize}–{MaximumTerminalFontSize} pt · Ctrl + mouse wheel or Ctrl + / Ctrl -", FontSize = 11, Foreground = ThemeBrush("MutedForegroundBrush"), Margin = new Thickness(0, 4, 0, 0) });
        Grid.SetColumn(fontLabel, 0); fontGrid.Children.Add(fontLabel);
        var fontCombo = new ComboBox { Name = "TerminalFontSizeComboBox", ItemsSource = Enumerable.Range(MinimumTerminalFontSize, MaximumTerminalFontSize - MinimumTerminalFontSize + 1), MinWidth = 90, ToolTip = "Choose the terminal font size" };
        fontCombo.SelectedItem = TerminalFontSize;
        fontCombo.SelectionChanged += async (_, _) =>
        {
            if (fontCombo.SelectedItem is int fontSize && !busy)
                await Run(() => SetTerminalFontSize(fontSize));
        };
        Grid.SetColumn(fontCombo, 1); fontGrid.Children.Add(fontCombo);
        fontSection.Child = fontGrid; content.Children.Add(fontSection);

        var historySection = new Border { Background = ThemeBrush("PanelBackgroundBrush"), BorderBrush = ThemeBrush("BorderBrush"), BorderThickness = new Thickness(1), Padding = new Thickness(16), Margin = new Thickness(0, 0, 0, 24) };
        var historyGrid = new Grid();
        historyGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        historyGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        var historyLabel = new StackPanel();
        historyLabel.Children.Add(new TextBlock { Text = "Saved terminal history", FontSize = 14, Foreground = ThemeBrush("ForegroundBrush") });
        historyLabel.Children.Add(new TextBlock { Text = "Encrypted for this Windows user and restored per pane after runtime restart", FontSize = 11, Foreground = ThemeBrush("MutedForegroundBrush"), Margin = new Thickness(0, 4, 0, 0), TextWrapping = TextWrapping.Wrap });
        Grid.SetColumn(historyLabel, 0); historyGrid.Children.Add(historyLabel);
        var historyChoices = TerminalHistoryChoices.Select(lines => new HistoryChoice(lines, lines == 0 ? "Off" : $"{lines:N0} lines")).ToArray();
        var historyCombo = new ComboBox { Name = "TerminalHistoryLinesComboBox", ItemsSource = historyChoices, DisplayMemberPath = "Label", MinWidth = 120, ToolTip = "Choose how many terminal history lines are saved" };
        historyCombo.SelectedItem = historyChoices.First(choice => choice.Lines == TerminalHistoryLines);
        historyCombo.SelectionChanged += async (_, _) =>
        {
            if (historyCombo.SelectedItem is HistoryChoice choice && !busy)
                await Run(() => SetTerminalHistoryLines(choice.Lines));
        };
        Grid.SetColumn(historyCombo, 1); historyGrid.Children.Add(historyCombo);
        historySection.Child = historyGrid; content.Children.Add(historySection);

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
    private Task SetTerminalHistoryLines(int lines) => Change("settings.set", new { key = "terminalHistoryLines", value = lines }, false);
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
            window.Content = panel;
            window.PreviewKeyDown += (_, e) => { if (e.Key == Key.Escape) { window.Close(); e.Handled = true; } };
            window.Loaded += (_, _) => { input.Focus(); input.SelectAll(); };
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
    private Task CycleTab(int direction)
    {
        if (ActiveWorkspace is not { } workspace || workspace.Tabs.Count == 0) return Task.CompletedTask;
        var index = workspace.Tabs.FindIndex(tab => tab.Id == workspace.ActiveTabId);
        if (index < 0) index = 0;
        index = (index + workspace.Tabs.Count + direction) % workspace.Tabs.Count;
        return Change("tab.focus", new { tabId = workspace.Tabs[index].Id });
    }
    private async Task Execute(string action)
    {
        switch (action)
        {
            case "Terminal.NewTab": await NewTab(); break;
            case "Terminal.NextTab": await CycleTab(1); break;
            case "Terminal.PreviousTab": await CycleTab(-1); break;
            case "Terminal.Focus": if (ActiveTab is { } focusTab) await FocusPane(focusTab.ActivePaneId); break;
            case "Terminal.CloseTab": if (ActiveTab is { } closeTab) await Change("tab.close", new { tabId = closeTab.Id }); break;
            case "Terminal.IncreaseFontSize": await AdjustTerminalFontSize(1); break;
            case "Terminal.DecreaseFontSize": await AdjustTerminalFontSize(-1); break;
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
            case "Help.ShowShortcuts": ShowShortcuts(); break;
            case "Terminal.Reconnect": state = await client.State(); Render(); break;
            default:
                if (ActiveTab is not { } active) break;
                if (action.StartsWith("Terminal.MoveFocus") && LayoutGeometry.Neighbor(active.RootLayoutNode, active.ActivePaneId, action[18..]) is { } neighbor) await FocusPane(neighbor);
                if (action.StartsWith("Terminal.ResizePane") && LayoutGeometry.Resize(active.RootLayoutNode, active.ActivePaneId, action[19..]) is { } resize) await Change("pane.resize", new { tabId = active.Id, path = resize.Path, ratio = resize.Ratio });
                break;
        }
    }
    private void TerminalNavigationMessage(ref MSG message, ref bool handled)
    {
        if (handled || !IsActive || dialog || showingSettings) return;
        if (message.message is not (0x100 or 0x101) || message.wParam.ToInt64() is < 0x25 or > 0x28) return;
        if (PressedModifierState(pressedModifierKeys) != ModifierKeys.None) return;
        foreach (var control in controls.Values)
        {
            var container = FindVisualChild<TerminalContainer>(control);
            if (container is null || message.hwnd != container.Handle || !HasTerminalFocus(control)) continue;
            handled = true;
            DispatchMessage(ref message);
            return;
        }
    }
    private void KeyMessage(ref MSG message, ref bool handled)
    {
        if (message.message is not (0x100 or 0x101 or 0x104 or 0x105)) return;
        var virtualKey = (int)message.wParam;
        var keyDown = message.message is 0x100 or 0x104;
        if (virtualKey is 0x10 or 0x11 or 0x12 or 0xA0 or 0xA1 or 0xA2 or 0xA3 or 0xA4 or 0xA5)
        {
            if (keyDown) pressedModifierKeys.Add(virtualKey);
            else pressedModifierKeys.Remove(virtualKey);
            return;
        }
        if (!keyDown || handled || !IsActive || dialog) return;
        var key = KeyInterop.KeyFromVirtualKey((int)message.wParam);
        var modifiers = PressedModifierState(pressedModifierKeys);
        var controlDown = (modifiers & ModifierKeys.Control) != 0;
        var altDown = (modifiers & ModifierKeys.Alt) != 0;
        var shiftDown = (modifiers & ModifierKeys.Shift) != 0;
        if (!HasShortcutModifier(controlDown, altDown, shiftDown)) return;
        if (key is Key.Left or Key.Right or Key.Up or Key.Down && !altDown) return;
        string? action = null;
        if (modifiers == (ModifierKeys.Control | ModifierKeys.Shift)) action = key switch { Key.T => "Terminal.NewTab", Key.W => "Terminal.ClosePane", Key.P => "Palette", _ => null };
        if (modifiers == ModifierKeys.Alt && key is Key.Left or Key.Right or Key.Up or Key.Down) action = "Terminal.MoveFocus" + key;
        if (modifiers == (ModifierKeys.Alt | ModifierKeys.Shift)) action = key switch
        {
            Key.D => "Terminal.SplitPaneAuto", Key.OemMinus => "Terminal.SplitPaneDown", Key.OemPlus => "Terminal.SplitPaneRight",
            Key.Left or Key.Right or Key.Up or Key.Down => "Terminal.ResizePane" + key, _ => null
        };
        if (action is null) action = NewShortcutAction(key, modifiers);
        if (action is null) return;
        handled = true;
        AppLogger.Info("app.shortcut", $"key={key} modifiers={modifiers} action={action}");
        var selected = action;
        Dispatcher.BeginInvoke(async () => { if (selected == "Palette") Commands(); else await Run(() => Execute(selected)); });
    }
    private static bool HasShortcutModifier(bool control, bool alt, bool shift) => control || alt || shift;
    private static ModifierKeys PressedModifierState(IReadOnlySet<int> pressedKeys) => ModifierState(
        pressedKeys.Contains(0x11) || pressedKeys.Contains(0xA2) || pressedKeys.Contains(0xA3),
        pressedKeys.Contains(0x12) || pressedKeys.Contains(0xA4) || pressedKeys.Contains(0xA5),
        pressedKeys.Contains(0x10) || pressedKeys.Contains(0xA0) || pressedKeys.Contains(0xA1));
    private static ModifierKeys ModifierState(bool control, bool alt, bool shift)
    {
        var modifiers = ModifierKeys.None;
        if (control) modifiers |= ModifierKeys.Control;
        if (alt) modifiers |= ModifierKeys.Alt;
        if (shift) modifiers |= ModifierKeys.Shift;
        return modifiers;
    }
    private static string? NewShortcutAction(Key key, ModifierKeys modifiers) => modifiers switch
    {
        ModifierKeys.Control => key switch
        {
            Key.Tab => "Terminal.NextTab", Key.Oem3 => "Terminal.Focus", Key.T => "Terminal.NewTab", Key.W => "Terminal.CloseTab", Key.N => "Workspace.New",
            Key.OemPlus or Key.Add => "Terminal.IncreaseFontSize", Key.OemMinus or Key.Subtract => "Terminal.DecreaseFontSize", _ => null
        },
        ModifierKeys.Control | ModifierKeys.Shift => key switch
        {
            Key.Tab => "Terminal.PreviousTab", Key.OemQuestion => "Help.ShowShortcuts",
            Key.OemPlus or Key.Add => "Terminal.IncreaseFontSize", Key.OemMinus or Key.Subtract => "Terminal.DecreaseFontSize", _ => null
        },
        ModifierKeys.Control | ModifierKeys.Alt => key switch
        {
            Key.Tab => "Workspace.Next", Key.R => "Workspace.Rename", _ => null
        },
        ModifierKeys.Control | ModifierKeys.Alt | ModifierKeys.Shift => key == Key.Tab ? "Workspace.Previous" : null,
        _ => null
    };
    private void ShowShortcuts()
    {
        dialog = true;
        try
        {
            var shortcuts = new (string Shortcut, string Action)[]
            {
                ("Ctrl+Tab", "Next tab"),
                ("Ctrl+Shift+Tab", "Previous tab"),
                ("Ctrl+Alt+Tab", "Next workspace"),
                ("Ctrl+Alt+Shift+Tab", "Previous workspace"),
                ("Ctrl+Alt+R", "Rename workspace"),
                ("Ctrl+T", "New tab"),
                ("Ctrl+W", "Close current tab"),
                ("Ctrl+N", "New workspace"),
                ("Ctrl+`", "Focus terminal pane"),
                ("Ctrl++", "Increase terminal font size"),
                ("Ctrl+-", "Decrease terminal font size"),
                ("Ctrl+Mouse Wheel", "Change terminal font size"),
                ("Ctrl+?", "Show keyboard shortcuts"),
                ("Ctrl+Shift+T", "New tab"),
                ("Ctrl+Shift+W", "Close focused pane"),
                ("Ctrl+Shift+P", "Command palette"),
                ("Alt+Shift+D", "Automatic split direction"),
                ("Alt+Shift+-", "Split pane down"),
                ("Alt+Shift++", "Split pane right"),
                ("Alt+Arrow", "Move focus between panes"),
                ("Alt+Shift+Arrow", "Resize the nearest split")
            };
            var window = new Window { Owner = this, Title = "Keyboard Shortcuts", Width = 620, Height = 560, WindowStartupLocation = WindowStartupLocation.CenterOwner, Background = ThemeBrush("SidebarBackgroundBrush"), Foreground = ThemeBrush("ForegroundBrush") };
            var root = new DockPanel { Margin = new Thickness(18) };
            var close = new Button { Content = "Close", IsCancel = true, HorizontalAlignment = HorizontalAlignment.Right, Padding = new Thickness(18, 6, 18, 6) };
            close.Click += (_, _) => window.Close();
            DockPanel.SetDock(close, Dock.Bottom);
            root.Children.Add(close);
            var list = new StackPanel();
            foreach (var shortcut in shortcuts)
            {
                var row = new Grid { Margin = new Thickness(0, 0, 0, 10) };
                row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(190) });
                row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                var shortcutText = new TextBlock { Text = shortcut.Shortcut, Foreground = ThemeBrush("AccentBrush"), FontFamily = new System.Windows.Media.FontFamily("Cascadia Mono") };
                var actionText = new TextBlock { Text = shortcut.Action, Foreground = ThemeBrush("ForegroundBrush") };
                Grid.SetColumn(shortcutText, 0); Grid.SetColumn(actionText, 1);
                row.Children.Add(shortcutText); row.Children.Add(actionText); list.Children.Add(row);
            }
            var scroll = new ScrollViewer { VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Content = list };
            root.Children.Add(scroll);
            window.Content = root;
            window.PreviewKeyDown += (_, e) => { if (e.Key == Key.Escape) { window.Close(); e.Handled = true; } };
            window.ShowDialog();
        }
        finally { dialog = false; }
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
                ["Terminal: Focus Terminal"] = "Terminal.Focus",
                ["Terminal: Next Tab"] = "Terminal.NextTab",
                ["Terminal: Previous Tab"] = "Terminal.PreviousTab",
                ["Terminal: Close Tab"] = "Terminal.CloseTab",
                ["Terminal: Increase Font Size"] = "Terminal.IncreaseFontSize",
                ["Terminal: Decrease Font Size"] = "Terminal.DecreaseFontSize",
                ["Terminal: Split Right"] = "Terminal.SplitPaneRight",
                ["Terminal: Split Down"] = "Terminal.SplitPaneDown",
                ["Terminal: Close Pane"] = "Terminal.ClosePane",
                ["Terminal: Rename Tab"] = "Terminal.OpenTabRenamer",
                ["Preferences: Open Settings"] = "Preferences.Settings",
                ["Workspace: Open Folder"] = "Workspace.New",
                ["Workspace: New Workspace"] = "Workspace.New",
                ["Workspace: Rename Workspace"] = "Workspace.Rename",
                ["Workspace: Change Workspace Folder"] = "Workspace.ChangeRoot",
                ["Workspace: Open Folder in Explorer"] = "Workspace.OpenRoot",
                ["Workspace: Next"] = "Workspace.Next",
                ["Workspace: Previous"] = "Workspace.Previous",
                ["Workspace: Close"] = "Workspace.Close",
                ["Help: Keyboard Shortcuts"] = "Help.ShowShortcuts",
                ["Paneacea: Reconnect Runtime"] = "Terminal.Reconnect"
            };
            var shortcutMap = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                ["Terminal: New Tab"] = "Ctrl+T / Ctrl+Shift+T",
                ["Terminal: Focus Terminal"] = "Ctrl+`",
                ["Terminal: Next Tab"] = "Ctrl+Tab",
                ["Terminal: Previous Tab"] = "Ctrl+Shift+Tab",
                ["Terminal: Close Tab"] = "Ctrl+W",
                ["Terminal: Increase Font Size"] = "Ctrl++",
                ["Terminal: Decrease Font Size"] = "Ctrl+-",
                ["Terminal: Split Right"] = "Alt+Shift++",
                ["Terminal: Split Down"] = "Alt+Shift+-",
                ["Terminal: Close Pane"] = "Ctrl+Shift+W",
                ["Workspace: Open Folder"] = "Ctrl+N",
                ["Workspace: New Workspace"] = "Ctrl+N",
                ["Workspace: Rename Workspace"] = "Ctrl+Alt+R",
                ["Workspace: Next"] = "Ctrl+Alt+Tab",
                ["Workspace: Previous"] = "Ctrl+Alt+Shift+Tab",
                ["Help: Keyboard Shortcuts"] = "Ctrl+?"
            };
            string[] actions = [.. commandMap.Keys];
            CommandPaletteItem CreateCommandPaletteItem(string action) => new(action, shortcutMap.TryGetValue(action, out var shortcut) ? shortcut : "—");
            var commandItems = actions.Select(CreateCommandPaletteItem).ToArray();
            var window = new Window { Owner = this, Title = "Command Palette", Width = 460, Height = 450, WindowStartupLocation = WindowStartupLocation.CenterOwner, Background = ThemeBrush("SidebarBackgroundBrush"), Foreground = ThemeBrush("ForegroundBrush") };
            var panel = new DockPanel { Margin = new Thickness(12) };
            var filter = new TextBox(); DockPanel.SetDock(filter, Dock.Top);
            var list = new ListBox { ItemsSource = commandItems, SelectedIndex = 0 };
            var itemTemplate = new DataTemplate();
            var row = new FrameworkElementFactory(typeof(DockPanel));
            row.SetValue(DockPanel.MarginProperty, new Thickness(6, 4, 6, 4));
            var shortcut = new FrameworkElementFactory(typeof(TextBlock));
            shortcut.SetValue(DockPanel.DockProperty, Dock.Right);
            shortcut.SetValue(TextBlock.ForegroundProperty, ThemeBrush("MutedForegroundBrush"));
            shortcut.SetValue(TextBlock.FontFamilyProperty, new FontFamily("Cascadia Mono"));
            shortcut.SetValue(TextBlock.MarginProperty, new Thickness(18, 0, 4, 0));
            shortcut.SetBinding(TextBlock.TextProperty, new Binding(nameof(CommandPaletteItem.Shortcut)));
            var label = new FrameworkElementFactory(typeof(TextBlock));
            label.SetValue(TextBlock.ForegroundProperty, ThemeBrush("ForegroundBrush"));
            label.SetBinding(TextBlock.TextProperty, new Binding(nameof(CommandPaletteItem.Label)));
            row.AppendChild(shortcut); row.AppendChild(label);
            itemTemplate.VisualTree = row; list.ItemTemplate = itemTemplate;
            filter.TextChanged += (_, _) => { list.ItemsSource = actions.Where(a => a.Contains(filter.Text, StringComparison.OrdinalIgnoreCase)).Select(CreateCommandPaletteItem).ToArray(); list.SelectedIndex = 0; };
            void Choose() { if (list.SelectedItem is CommandPaletteItem item && commandMap.TryGetValue(item.Label, out var command)) { selected = command; window.DialogResult = true; } }
            list.MouseDoubleClick += (_, _) => Choose();
            window.PreviewKeyDown += (_, e) => { if (e.Key == Key.Enter) { Choose(); e.Handled = true; } if (e.Key == Key.Escape) { window.Close(); e.Handled = true; } };
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
    private async void RenameWorkspaceContextMenu_Click(object sender, RoutedEventArgs e)
    {
        if ((sender as FrameworkElement)?.DataContext is not Workspace workspace) return;
        await Run(async () =>
        {
            if (Prompt("Rename workspace", "Name", workspace.Name) is { } name)
                await Change("workspace.rename", new { workspaceId = workspace.Id, name });
        });
    }
    private async void CloseWorkspaceContextMenu_Click(object sender, RoutedEventArgs e)
    {
        if ((sender as FrameworkElement)?.DataContext is not Workspace workspace) return;
        await Run(() => Change("workspace.close", new { workspaceId = workspace.Id }));
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
