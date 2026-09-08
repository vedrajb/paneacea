using System.Diagnostics;
using System.IO;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Input;
using System.Windows.Media;
using Microsoft.Terminal.Wpf;
using Paneacea;
using Paneacea.Core;

internal static class Program
{
    [STAThread]
    private static int Main()
    {
        var name = "paneacea-ui-test-" + Guid.NewGuid().ToString("N");
        Environment.SetEnvironmentVariable("PANEACEA_PIPE", name);
        var directory = Path.Combine(Environment.CurrentDirectory, ".data", name);
        Directory.CreateDirectory(directory);
        var start = new ProcessStartInfo(Path.Combine(Environment.CurrentDirectory, "target", "debug", "panacea-runtime.exe")) { UseShellExecute = false, CreateNoWindow = true };
        start.ArgumentList.Add("--pipe"); start.ArgumentList.Add(name); start.ArgumentList.Add("--data"); start.ArgumentList.Add(directory);
        using var daemon = Process.Start(start)!;
        var application = new Application { ShutdownMode = ShutdownMode.OnExplicitShutdown };
        application.Resources.MergedDictionaries.Add(new ResourceDictionary { Source = new Uri("/Paneacea.App;component/Themes/VSCodeDark.xaml", UriKind.Relative) });
        Check(application.Resources["EditorBackgroundBrush"] is SolidColorBrush editor && editor.Color == Color.FromRgb(0x1E, 0x1E, 0x1E), "VS Code editor theme resource");
        Check(application.Resources["SidebarBackgroundBrush"] is SolidColorBrush sidebar && sidebar.Color == Color.FromRgb(0x25, 0x25, 0x26), "VS Code Explorer theme resource");
        var result = 1;
        application.DispatcherUnhandledException += (_, e) => { Console.Error.WriteLine(e.Exception); e.Handled = true; application.Shutdown(1); };
        application.Startup += async (_, _) =>
        {
            MainWindow? window = null;
            try
            {
                var client = new RuntimeClient(name);
                for (var attempt = 0; ; attempt++)
                {
                    try { await client.State(); break; }
                    catch when (attempt < 30) { await Task.Delay(100); }
                }
                window = new MainWindow { Left = -10000, Top = -10000, ShowInTaskbar = false, ShowActivated = true };
                window.Show();
                Check(window.Title == "Paneacea", "Paneacea window title");
                Check(window.WindowStyle == WindowStyle.None, "custom dark title bar removes system caption");
                var minimizeButton = window.FindName("MinimizeWindowButton") as Button;
                Check(minimizeButton is not null, "custom minimize button");
                minimizeButton!.ApplyTemplate();
                Check(FindVisualChild<ContentPresenter>(minimizeButton, "") is { RecognizesAccessKey: false }, "button text preserves underscores without access-key shortcuts");
                Check(window.FindName("MaximizeWindowButton") is Button, "custom maximize button");
                Check(window.FindName("CloseWindowButton") is Button, "custom close button");
                Check(window.FindName("SettingsButton") is Button, "settings activity button");
                var controls = (Dictionary<string, TerminalControl>)typeof(MainWindow).GetField("controls", BindingFlags.Instance | BindingFlags.NonPublic)!.GetValue(window)!;
                var status = (TextBlock)window.FindName("Status");
                for (var attempt = 0; controls.Count == 0 && attempt < 200; attempt++) await Task.Delay(100);
                Check(controls.Count == 1, "initial terminal: " + status.Text);
                await Task.Delay(150);
                var startupControl = controls.Values.Single();
                Check(HasNativeFocus(startupControl), "startup places native keyboard focus in the terminal pane");
                var paneBorders = (Dictionary<string, Border>)typeof(MainWindow).GetField("paneBorders", BindingFlags.Instance | BindingFlags.NonPublic)!.GetValue(window)!;
                Check(paneBorders.Values.Single().BorderThickness.Left == 2, "startup highlights the focused terminal pane");
                Check(controls.Values.First().Rows > 0, "native renderer dimensions");
                var terminalScrollBar = FindVisualChildren<ScrollBar>(controls.Values.First()).SingleOrDefault(scrollbar => scrollbar.Orientation == Orientation.Vertical);
                Check(terminalScrollBar is not null && terminalScrollBar.Width == 4, "terminal scrollbar is a thin idle indicator");
                var terminalThumb = terminalScrollBar is null ? null : FindVisualChild<Thumb>(terminalScrollBar, "");
                Check(terminalThumb is not null && terminalThumb.Opacity < 1, "terminal scrollbar thumb is dimmed when idle");
                Check(terminalScrollBar is not null && FindVisualChildren<RepeatButton>(terminalScrollBar).All(button => button.Visibility == Visibility.Collapsed), "terminal scrollbar removes end buttons");
                terminalScrollBar!.RaiseEvent(new MouseEventArgs(Mouse.PrimaryDevice, 0) { RoutedEvent = Mouse.MouseEnterEvent });
                Check(terminalScrollBar.Width == 16, "terminal scrollbar expands on hover");
                terminalScrollBar.RaiseEvent(new MouseEventArgs(Mouse.PrimaryDevice, 0) { RoutedEvent = Mouse.MouseLeaveEvent });
                Check(terminalScrollBar.Width == 4, "terminal scrollbar returns to its thin idle state");
                async Task Execute(string action) => await (Task)typeof(MainWindow).GetMethod("Execute", BindingFlags.Instance | BindingFlags.NonPublic)!.Invoke(window, [action])!;
                await Execute("Preferences.Settings");
                await Task.Delay(150);
                var paneHost = (ContentControl)window.FindName("PaneHost");
                var shellCombo = FindVisualChild<ComboBox>(paneHost, "DefaultShellComboBox");
                Check(shellCombo is not null && shellCombo.Items.Count > 0, "settings panel detects available shells");
                Check(shellCombo!.Background is SolidColorBrush comboBackground && comboBackground.Color == Color.FromRgb(0x1E, 0x1E, 0x1E), "settings shell picker uses dark theme");
                Check(shellCombo.SelectedItem is ShellProfile selectedShell && selectedShell.IsAvailable, "settings panel selects an available default shell");
                var fontCombo = FindVisualChild<ComboBox>(paneHost, "TerminalFontSizeComboBox");
                Check(fontCombo is not null && fontCombo.Items.Count == 25, "settings panel exposes terminal font sizes");
                Check(fontCombo!.SelectedItem is int selectedFontSize && selectedFontSize == 11, "settings panel defaults terminal font size to 11");
                fontCombo.SelectedItem = 12;
                WorkspaceState? fontState = null;
                for (var attempt = 0; attempt < 100; attempt++)
                {
                    fontState = await client.State();
                    if (fontState.Settings.TryGetValue("terminalFontSize", out var setting)
                        && setting.ValueKind == System.Text.Json.JsonValueKind.Number
                        && setting.GetInt32() == 12) break;
                    await Task.Delay(50);
                }
                Check(fontState is not null
                    && fontState.Settings.TryGetValue("terminalFontSize", out var savedFontSize)
                    && savedFontSize.ValueKind == System.Text.Json.JsonValueKind.Number
                    && savedFontSize.GetInt32() == 12, "terminal font size selection persists");
                fontCombo.SelectedItem = 11;
                for (var attempt = 0; attempt < 100; attempt++)
                {
                    var restoredFontState = await client.State();
                    if (restoredFontState.Settings.TryGetValue("terminalFontSize", out var setting)
                        && setting.ValueKind == System.Text.Json.JsonValueKind.Number
                        && setting.GetInt32() == 11) break;
                    await Task.Delay(50);
                }
                var gitShell = shellCombo.Items.Cast<ShellProfile>().SingleOrDefault(shell => shell.Id == "git-bash");
                if (gitShell is { IsAvailable: true } && shellCombo.SelectedItem is ShellProfile originalShell)
                {
                    shellCombo.SelectedItem = gitShell;
                    WorkspaceState? selectedState = null;
                    for (var attempt = 0; attempt < 100; attempt++)
                    {
                        selectedState = await client.State();
                        if (selectedState.Settings.TryGetValue("defaultShell", out var setting)
                            && setting.ValueKind == System.Text.Json.JsonValueKind.Object
                            && setting.TryGetProperty("id", out var id)
                            && string.Equals(id.GetString(), gitShell.Id, StringComparison.OrdinalIgnoreCase)) break;
                        await Task.Delay(50);
                    }
                    Check(selectedState is not null
                        && selectedState.Settings.TryGetValue("defaultShell", out var savedSetting)
                        && savedSetting.ValueKind == System.Text.Json.JsonValueKind.Object
                        && savedSetting.TryGetProperty("id", out var savedId)
                        && string.Equals(savedId.GetString(), gitShell.Id, StringComparison.OrdinalIgnoreCase), "Git Bash selection persists");
                    await Execute("Terminal.NewTab");
                    await Task.Delay(250);
                    var gitState = await client.State();
                    var gitWorkspace = gitState.Workspaces.Single(workspace => workspace.Id == gitState.ActiveWorkspaceId);
                    var gitTab = gitWorkspace.Tabs.Single(tab => tab.Id == gitWorkspace.ActiveTabId);
                    Check(gitState.Panes[gitTab.ActivePaneId].Executable.Equals(gitShell.Executable, StringComparison.OrdinalIgnoreCase), "new tab uses selected Git Bash");
                    await Execute("Terminal.ClosePane");
                    await Execute("Preferences.Settings");
                    await Task.Delay(150);
                    var restoredCombo = FindVisualChild<ComboBox>((ContentControl)window.FindName("PaneHost"), "DefaultShellComboBox");
                    Check(restoredCombo is not null, "settings panel reopens after shell test");
                    restoredCombo!.SelectedItem = originalShell;
                    await Task.Delay(150);
                }
                var backButton = FindVisualChild<Button>(paneHost, "SettingsBackButton");
                Check(backButton is not null, "settings panel returns to terminal");
                backButton!.RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
                for (var attempt = 0; controls.Count == 0 && attempt < 100; attempt++) await Task.Delay(50);
                Check(controls.Count == 1, "settings panel preserves terminal workspace");
                var promptMethod = typeof(MainWindow).GetMethod("Prompt", BindingFlags.Instance | BindingFlags.NonPublic)!;
                var promptTask = window.Dispatcher.InvokeAsync(() => promptMethod.Invoke(window, ["Workspace root", "Existing absolute directory", Environment.CurrentDirectory, true])).Task;
                Window? promptWindow = null;
                Button? browseButton = null;
                for (var attempt = 0; attempt < 100 && browseButton is null; attempt++)
                {
                    promptWindow = Application.Current.Windows.OfType<Window>().FirstOrDefault(candidate => candidate != window);
                    if (promptWindow is not null) browseButton = FindVisualChild<Button>(promptWindow, "BrowseButton");
                    if (browseButton is null) await Task.Delay(50);
                }
                Check(browseButton is not null, "workspace root prompt has Explorer browse button");
                promptWindow!.DialogResult = false;
                await promptTask;
                var connections = (List<PipeTerminalConnection>)typeof(MainWindow).GetField("connections", BindingFlags.Instance | BindingFlags.NonPublic)!.GetValue(window)!;
                var output = new StringBuilder();
                connections[0].TerminalOutput += (_, e) => { lock (output) output.Append(e.Data); };
                connections[0].WriteInput("Write-Output ('GUI-' + 'INPUT-OK')\r");
                var received = false;
                for (var attempt = 0; attempt < 100; attempt++)
                {
                    lock (output) received = output.ToString().Contains("GUI-INPUT-OK");
                    if (received) break;
                    await Task.Delay(100);
                }
                Check(received, "terminal connection forwards input and streams output");
                var before = await client.State();
                var paneId = before.Panes.Keys.Single();
                await Execute("Terminal.SplitPaneRight");
                await Task.Delay(250);
                Check(controls.Count == 2, "split renders two controls");
                var splitPaneId = controls.Keys.Single(paneId => paneId != before.Panes.Keys.Single());
                var splitControl = controls[splitPaneId];
                splitControl.RaiseEvent(new MouseButtonEventArgs(Mouse.PrimaryDevice, 0, MouseButton.Left) { RoutedEvent = Mouse.PreviewMouseDownEvent });
                await Task.Delay(250);
                Check(HasNativeFocus(splitControl), "mouse-selected pane receives native keyboard focus");
                Check(paneBorders[splitPaneId].BorderThickness.Left == 2, "mouse-selected pane receives the focus highlight");
                await Execute("Terminal.ResizePaneRight");
                await Execute("Terminal.MoveFocusLeft");
                await Execute("Terminal.NewTab");
                Check(controls.Count == 1, "hidden tab renderers are detached");
                await Execute("Terminal.ClosePane");
                Check(controls.Count == 2, "closing final pane restores previous tab");
                await Execute("Terminal.Reconnect");
                await Task.Delay(250);
                Check(controls.Count == 2, "reattach rebuilds native controls");
                window.Close(); window = null;
                var after = await client.State();
                Check(after.Panes.ContainsKey(paneId) && after.Panes.Count == 2, "closing GUI preserves runtime sessions");
                window = new MainWindow { Left = -10000, Top = -10000, ShowInTaskbar = false, ShowActivated = true };
                window.Show();
                var restoredControls = (Dictionary<string, TerminalControl>)typeof(MainWindow).GetField("controls", BindingFlags.Instance | BindingFlags.NonPublic)!.GetValue(window)!;
                for (var attempt = 0; restoredControls.Count < 2 && attempt < 200; attempt++) await Task.Delay(100);
                var restoredWorkspace = after.Workspaces.Single(workspace => workspace.Id == after.ActiveWorkspaceId);
                var restoredTab = restoredWorkspace.Tabs.Single(tab => tab.Id == restoredWorkspace.ActiveTabId);
                await Task.Delay(750);
                Check(restoredControls.Count == 2, "restored multi-pane layout renders both controls");
                Check(HasNativeFocus(restoredControls[restoredTab.ActivePaneId]), "restored multi-pane startup focuses the active terminal cursor");
                window.Close(); window = null;
                foreach (var workspace in after.Workspaces) await client.Call("workspace.close", new { workspaceId = workspace.Id });
                Console.WriteLine("PASS WPF native renderer, split, resize, focus, tabs, reconnect, and GUI detach");
                result = 0;
            }
            catch (Exception error) { Console.Error.WriteLine(error); }
            finally { window?.Close(); application.Shutdown(result); }
        };
        try { application.Run(); }
        finally { if (!daemon.HasExited) daemon.Kill(); daemon.WaitForExit(); }
        return result;
    }
    [DllImport("user32.dll")]
    private static extern IntPtr GetFocus();
    [DllImport("user32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool IsChild(IntPtr parent, IntPtr child);
    private static bool HasNativeFocus(TerminalControl control)
    {
        var container = FindVisualChildren<TerminalContainer>(control).SingleOrDefault();
        var focused = GetFocus();
        return container is not null && focused != IntPtr.Zero && (focused == container.Handle || IsChild(container.Handle, focused));
    }
    private static void Check(bool condition, string name) { if (!condition) throw new Exception("FAIL " + name); }
    private static T? FindVisualChild<T>(DependencyObject root, string name) where T : FrameworkElement
    {
        if (root is T element && element.Name == name) return element;
        for (var index = 0; index < System.Windows.Media.VisualTreeHelper.GetChildrenCount(root); index++)
            if (FindVisualChild<T>(System.Windows.Media.VisualTreeHelper.GetChild(root, index), name) is { } child) return child;
        return null;
    }
    private static IEnumerable<T> FindVisualChildren<T>(DependencyObject root) where T : FrameworkElement
    {
        if (root is T element) yield return element;
        for (var index = 0; index < System.Windows.Media.VisualTreeHelper.GetChildrenCount(root); index++)
            foreach (var child in FindVisualChildren<T>(System.Windows.Media.VisualTreeHelper.GetChild(root, index))) yield return child;
    }
}
