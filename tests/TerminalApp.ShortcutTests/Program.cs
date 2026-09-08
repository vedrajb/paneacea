using System.Reflection;
using System.Windows.Input;
using TinkerShell;

var method = typeof(MainWindow).GetMethod("NewShortcutAction", BindingFlags.Static | BindingFlags.NonPublic)!;
Check(Action(Key.Tab, ModifierKeys.Control) == "Terminal.NextTab", "Ctrl+Tab cycles forward through tabs");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Shift) == "Terminal.PreviousTab", "Ctrl+Shift+Tab cycles backward through tabs");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Alt) == "Workspace.Next", "Ctrl+Alt+Tab cycles forward through workspaces");
Check(Action(Key.Tab, ModifierKeys.Control | ModifierKeys.Alt | ModifierKeys.Shift) == "Workspace.Previous", "Ctrl+Alt+Shift+Tab cycles backward through workspaces");
Check(Action(Key.R, ModifierKeys.Control | ModifierKeys.Alt) == "Workspace.Rename", "Ctrl+Alt+R renames the workspace");
Check(Action(Key.T, ModifierKeys.Control) == "Terminal.NewTab", "Ctrl+T creates a tab");
Check(Action(Key.W, ModifierKeys.Control) == "Terminal.CloseTab", "Ctrl+W closes the current tab");
Check(Action(Key.N, ModifierKeys.Control) == "Workspace.New", "Ctrl+N creates a workspace");
Check(Action(Key.OemQuestion, ModifierKeys.Control | ModifierKeys.Shift) == "Help.ShowShortcuts", "Ctrl+Question opens shortcuts");
Check(Action(Key.Tab, ModifierKeys.None) is null, "plain Tab is not intercepted by the app");
Check(Action(Key.C, ModifierKeys.Control) is null, "Ctrl+C remains available to the terminal");
Check(Action(Key.V, ModifierKeys.Control) is null, "Ctrl+V remains available to the terminal");
Check(Action(Key.C, ModifierKeys.Control | ModifierKeys.Shift) is null, "Ctrl+Shift+C remains available to the terminal");
Check(Action(Key.V, ModifierKeys.Control | ModifierKeys.Shift) is null, "Ctrl+Shift+V remains available to the terminal");
Console.WriteLine("PASS shortcut mappings");

string? Action(Key key, ModifierKeys modifiers) => (string?)method.Invoke(null, [key, modifiers]);
static void Check(bool condition, string message) { if (!condition) throw new Exception("FAIL " + message); }
