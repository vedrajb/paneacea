import { expect, test } from "vitest";
import { workspaceMenuItems } from "./workspaceMenu";

test("workspace context menu exposes only rename and close", () => {
  expect(workspaceMenuItems).toEqual([
    { action: "Workspace.Rename", label: "Rename" },
    { action: "Workspace.Close", label: "Close" },
  ]);
});
