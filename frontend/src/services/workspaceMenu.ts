export type WorkspaceMenuAction = "Workspace.Rename" | "Workspace.Close";

export const workspaceMenuItems: readonly {
  action: WorkspaceMenuAction;
  label: string;
}[] = [
  { action: "Workspace.Rename", label: "Rename" },
  { action: "Workspace.Close", label: "Close" },
];
