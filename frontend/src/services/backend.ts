export type Layout = {
  paneId?: string;
  orientation?: "vertical" | "horizontal";
  ratio?: number;
  first?: Layout;
  second?: Layout;
};
export type Agent = {
  type: string;
  state: string;
  sessionId: string;
  rootPid: number;
  processGeneration: string;
  executable: string;
  arguments: string[];
};
export type Pane = {
  id: string;
  workspaceId: string;
  tabId: string;
  title: string;
  executable: string;
  runningProgram?: string;
  arguments: string[];
  currentWorkingDirectory: string;
  status: string;
  error?: string;
  pid: number;
  agent?: Agent;
};
export type Tab = {
  id: string;
  title: string;
  titleMode: string;
  rootLayoutNode: Layout;
  activePaneId: string;
};
export type Workspace = {
  id: string;
  name: string;
  rootDirectory: string;
  tabs: Tab[];
  activeTabId: string;
};
export type Profile = {
  id: string;
  name: string;
  executable: string;
  arguments: string[];
  available: boolean;
};
export type Settings = {
  defaultShell: Profile;
  scrollback: number;
  terminalHistoryLines: number;
  fontSize: number;
  theme: string;
  keybindings: Record<string, string>;
};
export type State = {
  revision: number;
  workspaces: Workspace[];
  panes: Record<string, Pane>;
  activeWorkspaceId: string;
  settings: Settings;
};
export type Output = {
  data: string | null;
  sequence: number;
  exited: boolean;
  truncated: boolean;
  snapshot: boolean;
  restored: boolean;
  viewportOffset: number;
  columns: number;
  rows: number;
};
type Bridge = {
  Call(method: string, params: unknown): Promise<unknown>;
  ReadOutput(streamID: string, id: string, sequence: number): Promise<Output>;
  Detach(id: string): Promise<void>;
  OpenFolder(): Promise<string>;
  Copy(text: string): Promise<void>;
};
declare global {
  interface Window {
    go?: { desktop?: { App?: Bridge } };
    runtime?: {
      WindowMinimise(): void;
      WindowToggleMaximise(): void;
      Quit(): void;
    };
  }
}
export function bridge(): Bridge {
  const app = window.go?.desktop?.App;
  if (!app) throw new Error("Open Paneacea with the desktop executable.");
  return app;
}
export async function call<T = State>(
  method: string,
  params: unknown = {},
): Promise<T> {
  return (await bridge().Call(method, params)) as T;
}
export function bytes(data: string | null): Uint8Array {
  return Uint8Array.from(atob(data ?? ""), (c) => c.charCodeAt(0));
}
