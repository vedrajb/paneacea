using System.Reflection;
using System.Windows.Input;
using Paneacea;
using Paneacea.Core;

var method = typeof(MainWindow).GetMethod("NewShortcutAction", BindingFlags.Static | BindingFlags.NonPublic)!;
Check(Action(Key.Tab, ModifierKeys.Control) == "Terminal.NextTab", "Ctrl+Tab cycles forward through tabs");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Shift) == "Terminal.PreviousTab", "Ctrl+Shift+Tab cycles backward through tabs");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Alt) == "Workspace.Next", "Ctrl+Alt+Tab cycles forward through workspaces");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Alt | ModifierKeys.Shift) == "Workspace.Previous", "Ctrl+Alt+Shift+Tab cycles backward through workspaces");
Check(Action(Key.R, ModifierKeys.Control | ModifierKeys.Alt) == "Workspace.Rename", "Ctrl+Alt+R renames the workspace");
Check(Action(Key.Oem3, ModifierKeys.Control) == "Terminal.Focus", "Ctrl+Backtick focuses the terminal pane");
Check(Action(Key.T, ModifierKeys.Control) == "Terminal.NewTab", "Ctrl+T creates a tab");
Check(Action(Key.W, ModifierKeys.Control) == "Terminal.CloseTab", "Ctrl+W closes the current tab");
Check(Action(Key.N, ModifierKeys.Control) == "Workspace.New", "Ctrl+N creates a workspace");
Check(Action(Key.OemPlus, ModifierKeys.Control) == "Terminal.IncreaseFontSize", "Ctrl+Plus increases terminal font size");
Check(Action(Key.Add, ModifierKeys.Control) == "Terminal.IncreaseFontSize", "Ctrl+NumpadPlus increases terminal font size");
Check(Action(Key.OemMinus, ModifierKeys.Control) == "Terminal.DecreaseFontSize", "Ctrl+Minus decreases terminal font size");
Check(Action(Key.Subtract, ModifierKeys.Control) == "Terminal.DecreaseFontSize", "Ctrl+NumpadMinus decreases terminal font size");
Check(Action(Key.OemQuestion, ModifierKeys.Control | ModifierKeys.Shift) == "Help.ShowShortcuts", "Ctrl+Question opens shortcuts");
Check(Action(Key.Tab, ModifierKeys.None) is null, "plain Tab is not intercepted by the app");
Check(Action(Key.C, ModifierKeys.Control) is null, "Ctrl+C remains available to the terminal");
Check(Action(Key.V, ModifierKeys.Control) is null, "Ctrl+V remains available to the terminal");
Check(Action(Key.C, ModifierKeys.Control | ModifierKeys.Shift) is null, "Ctrl+Shift+C remains available to the terminal");
Check(Action(Key.V, ModifierKeys.Control | ModifierKeys.Shift) is null, "Ctrl+Shift+V remains available to the terminal");
Check(Action(Key.M, ModifierKeys.None) is null, "plain M remains available to the terminal");
Check(Action(Key.M, ModifierKeys.Alt) is null, "Alt+M remains available to the terminal");
var terminalHeader = typeof(MainWindow).GetMethod("TerminalHeader", BindingFlags.Static | BindingFlags.NonPublic)!;
var shellHeader = (string)terminalHeader.Invoke(null, [new Pane { Title = @"C:\Program Files\Git\usr\bin\bash.exe", Executable = @"C:\Program Files\Git\usr\bin\bash.exe", CurrentWorkingDirectory = @"C:\workspace\paneacea" }])!;
Check(shellHeader == "paneacea | bash.exe", "shell pane headers show folder and command");
var appHeader = (string)terminalHeader.Invoke(null, [new Pane { Title = "OpenAI Codex", Executable = "pwsh.exe", CurrentWorkingDirectory = @"C:\workspace\paneacea" }])!;
Check(appHeader == "paneacea | OpenAI Codex", "app-specific pane headers show folder and app");
var underscoredHeader = (string)terminalHeader.Invoke(null, [new Pane { Title = @"C:\Program Files\Git\usr\bin\bash.exe", Executable = @"C:\Program Files\Git\usr\bin\bash.exe", CurrentWorkingDirectory = @"C:\workspace\_misc" }])!;
Check(underscoredHeader == "_misc | bash.exe", "pane headers preserve underscored folders");
Console.WriteLine("PASS shortcut mappings");

string? Action(Key key, ModifierKeys modifiers) => (string?)method.Invoke(null, [key, modifiers]);
static void Check(bool condition, string message) { if (!condition) throw new Exception("FAIL " + message); }
