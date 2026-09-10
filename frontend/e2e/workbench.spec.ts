import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const profile = {
      id: "powershell",
      name: "PowerShell",
      executable: "powershell.exe",
      arguments: [],
      available: true,
    };
    const state: any = {
      revision: 1,
      activeWorkspaceId: "w",
      settings: {
        defaultShell: profile,
        fontSize: 13,
        scrollback: 10000,
        theme: "dark",
        keybindings: {},
      },
      workspaces: [
        {
          id: "w",
          name: "Paneacea",
          rootDirectory: "C:\\dev\\paneacea",
          activeTabId: "t",
          tabs: [
            {
              id: "t",
              title: "PowerShell",
              titleMode: "automatic",
              activePaneId: "p",
              rootLayoutNode: { paneId: "p" },
            },
          ],
        },
      ],
      panes: {
        p: {
          id: "p",
          tabId: "t",
          workspaceId: "w",
          title: "PowerShell",
          status: "running",
          pid: 123,
          executable: "powershell.exe",
          currentWorkingDirectory: "C:\\dev\\paneacea",
        },
      },
    };
    const calls: any[] = [];
    state.workspaces.push({
      id: "other",
      name: "Other workspace",
      rootDirectory: "C:\\dev\\other",
      activeTabId: "other-tab",
      tabs: [
        {
          id: "other-tab",
          title: "PowerShell",
          titleMode: "automatic",
          activePaneId: "other-pane",
          rootLayoutNode: { paneId: "other-pane" },
        },
      ],
    });
    state.panes["other-pane"] = {
      ...state.panes.p,
      id: "other-pane",
      tabId: "other-tab",
      workspaceId: "other",
      pid: 125,
    };
    (window as any).testCalls = calls;
    const streams = new Set<string>();
    window.go = {
      desktop: {
        App: {
          Call: async (method: string, params: any) => {
            calls.push({ method, params });
            const w = state.workspaces.find(
                (entry: any) =>
                  entry.id === (params.workspaceId ?? state.activeWorkspaceId),
              ),
              tab = w.tabs.find(
                (t: any) => t.id === (params.tabId ?? w.activeTabId),
              );
            if (method === "profiles.list") return [profile];
            if (method === "workspace.switch")
              state.activeWorkspaceId = params.workspaceId;
            if (method === "workspace.rename") w.name = params.name;
            if (method === "workspace.create") {
              const id = "w" + state.workspaces.length;
              state.workspaces.push({
                id,
                name: params.name,
                rootDirectory: params.rootDirectory,
                activeTabId: "",
                tabs: [],
              });
              state.activeWorkspaceId = id;
            }
            if (method === "tab.create") {
              const id = "t" + state.revision,
                paneId = "p" + state.revision;
              state.panes[paneId] = {
                ...state.panes.p,
                id: paneId,
                tabId: id,
                workspaceId: w.id,
                pid: 130,
              };
              w.tabs.push({
                id,
                title: "PowerShell",
                titleMode: "automatic",
                activePaneId: paneId,
                rootLayoutNode: { paneId },
              });
              w.activeTabId = id;
            }
            if (method === "tab.focus") w.activeTabId = params.tabId;
            if (method === "tab.close") {
              w.tabs = w.tabs.filter((entry: any) => entry.id !== params.tabId);
              for (const [id, pane] of Object.entries(state.panes))
                if ((pane as any).tabId === params.tabId)
                  delete state.panes[id];
              w.activeTabId = w.tabs[0]?.id ?? "";
            }
            if (method === "pane.split") {
              const id = "p" + Object.keys(state.panes).length;
              state.panes[id] = { ...state.panes.p, id, pid: 124 };
              tab.rootLayoutNode = {
                orientation: params.orientation,
                ratio: 0.5,
                first: tab.rootLayoutNode,
                second: { paneId: id },
              };
              tab.activePaneId = id;
            }
            if (method === "pane.focus") tab.activePaneId = params.paneId;
            if (method === "tab.rename") {
              tab.title = params.title;
              tab.titleMode = "manual";
            }
            if (method === "settings.set") state.settings = params.settings;
            if (method === "pane.resize") {
              tab.rootLayoutNode.ratio = params.ratio;
            }
            if (method !== "state.get") state.revision++;
            return structuredClone(state);
          },
          ReadOutput: async (
            streamID: string,
            id: string,
            sequence: number,
          ) => {
            if (!streams.has(streamID)) {
              streams.add(streamID);
              return {
                data: btoa(
                  "\x1b[32mPaneacea runtime connected\x1b[0m\r\nPS C:\\dev\\paneacea> ",
                ),
                sequence: 100,
                exited: false,
                truncated: false,
                snapshot: true,
                columns: 100,
                rows: 25,
              };
            }
            await new Promise((r) => setTimeout(r, 10000));
            return {
              data: null,
              sequence,
              exited: false,
              truncated: false,
              snapshot: false,
              columns: 0,
              rows: 0,
            };
          },
          Detach: async (id: string) => {
            calls.push({ method: "detach", id });
          },
          OpenFolder: async () => {
            calls.push({ method: "folder.open" });
            return "C:\\dev";
          },
          Copy: async (text: string) => {
            calls.push({ method: "copy", text });
          },
        },
      },
    };
  });
});

test("renders terminal, splits, resizes, and renames a tab", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(page.getByRole("tab", { name: "Tab 01" })).toBeVisible();
  await expect(page.locator(".xterm")).toHaveCount(1);
  await page.getByTitle("Split right", { exact: true }).click();
  await expect(page.locator(".xterm")).toHaveCount(2);
  const splitter = page.getByRole("slider");
  await splitter.focus();
  await page.keyboard.press("ArrowLeft");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls.some(
          (c: any) => c.method === "pane.resize" && c.params.ratio === 0.45,
        ),
      ),
    )
    .toBe(true);
  await page.getByRole("tab", { name: "Tab 01" }).dblclick();
  await page.getByLabel("Title", { exact: true }).fill("Build & test");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("tab", { name: "Build & test" })).toBeVisible();
  await page.screenshot({
    path: "../.build-validation/paneacea-workbench.png",
  });
  expect(errors).toEqual([]);
});

test("terminal shortcuts work once and modal focus stays usable", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".xterm-helper-textarea")).toHaveCount(1);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Alt+Shift+d");
  await expect(page.locator(".xterm")).toHaveCount(2);
  expect(
    await page.evaluate(
      () =>
        (window as any).testCalls.filter((c: any) => c.method === "pane.split")
          .length,
    ),
  ).toBe(1);
  await page.getByTitle("Settings", { exact: true }).click();
  await page.getByLabel("Font size", { exact: true }).fill("16");
  await page
    .getByRole("button", { name: "Save settings", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Settings", exact: true }),
  ).toHaveCount(0);
  await page.getByTitle("Command palette", { exact: true }).click();
  await page.getByLabel("Search commands", { exact: true }).fill("rename tab");
  await expect(
    page.getByRole("button", { name: "Terminal: Rename Tab" }),
  ).toBeVisible();
});

test("legacy tab and workspace shortcuts perform their actions", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".xterm")).toHaveCount(1);
  await expect(page.locator(".xterm-helper-textarea")).toBeFocused();
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+t");
  await expect(page.getByRole("tab")).toHaveCount(2);
  await expect(page.getByRole("tab", { name: "Tab 02" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Control+Shift+Tab");
  await expect(page.getByRole("tab", { name: "Tab 01" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Control+Tab");
  await expect(page.getByRole("tab", { name: "Tab 02" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Alt+Shift+d");
  await expect(page.locator(".xterm")).toHaveCount(2);
  await page.keyboard.press("Control+w");
  await expect(page.getByRole("tab")).toHaveCount(1);
  await expect(page.locator(".xterm")).toHaveCount(1);
  await page.keyboard.press("Control+Alt+Tab");
  await expect(page.getByRole("tab", { name: "Tab 01" })).toBeVisible();
  await page.keyboard.press("Control+Alt+Shift+Tab");
  await expect(page.getByRole("tab", { name: "Tab 01" })).toBeVisible();
  await page.keyboard.press("Control+Alt+r");
  await page.getByLabel("Name", { exact: true }).fill("Renamed workspace");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.locator(".window-title")).toHaveText("Renamed workspace");
  await page.keyboard.press("Control+n");
  await page.getByLabel("Name", { exact: true }).fill("New workspace");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.locator(".window-title")).toHaveText("New workspace");
  const calls = await page.evaluate(() => (window as any).testCalls);
  expect(
    calls.filter((entry: any) => entry.method === "tab.close"),
  ).toHaveLength(1);
  expect(
    calls.filter((entry: any) => entry.method === "pane.close"),
  ).toHaveLength(0);
  expect(
    calls.filter((entry: any) => entry.method === "workspace.create"),
  ).toHaveLength(1);
});

test("legacy font keys and Ctrl+wheel resize terminals without remounting", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".xterm")).toHaveCount(1);
  await page.locator(".xterm-helper-textarea").focus();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls.filter(
            (entry: any) => entry.method === "terminal.resize",
          ).length,
      ),
    )
    .toBeGreaterThan(0);
  await page.keyboard.press("Control+Equal");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(14);
  await page.keyboard.press("Control+Shift+Equal");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(15);
  await page.keyboard.press("Control+NumpadAdd");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(16);
  await page.keyboard.press("Control+NumpadSubtract");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(15);
  await page
    .locator(".xterm-host")
    .dispatchEvent("wheel", { ctrlKey: true, deltaY: -120 });
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(16);
  await page.keyboard.press("Control+Minus");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls
            .filter((entry: any) => entry.method === "settings.set")
            .at(-1)?.params.settings.fontSize,
      ),
    )
    .toBe(15);
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls.filter(
            (entry: any) => entry.method === "terminal.resize",
          ).length,
      ),
    )
    .toBeGreaterThan(1);
  expect(
    await page.evaluate(() =>
      (window as any).testCalls.filter(
        (entry: any) => entry.method === "detach",
      ),
    ),
  ).toHaveLength(0);
});

test("focus shortcut returns from settings and shortcut help lists current bindings", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".xterm")).toHaveCount(1);
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.insertText("baseline");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls
          .filter((entry: any) => entry.method === "pane.sendInput")
          .map((entry: any) => entry.params.data)
          .join("")
          .includes("baseline"),
      ),
    )
    .toBe(true);
  await page.keyboard.press("Control+Backquote");
  await expect(page.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.type("focus-ok");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls
          .filter((entry: any) => entry.method === "pane.sendInput")
          .map((entry: any) => entry.params.data)
          .join("")
          .includes("focus-ok"),
      ),
    )
    .toBe(true);
  await page.getByTitle("Settings", { exact: true }).click();
  await page.getByLabel("Font size", { exact: true }).focus();
  await page.keyboard.press("Control+Backquote");
  await expect(
    page.getByRole("heading", { name: "Settings", exact: true }),
  ).toHaveCount(0);
  await expect(page.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.press("Control+Shift+Slash");
  const help = page.getByRole("dialog", { name: "Keyboard shortcuts" });
  await expect(help).toBeVisible();
  await expect(
    help.getByText("Ctrl+Alt+Shift+Tab", { exact: true }),
  ).toBeVisible();
  await expect(
    help.getByText("Ctrl+Mouse Wheel", { exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(help).toHaveCount(0);
  await expect(page.locator(".xterm-helper-textarea")).toBeFocused();
  await page.keyboard.press("Control+Shift+w");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as any).testCalls.filter(
            (entry: any) => entry.method === "pane.close",
          ).length,
      ),
    )
    .toBe(1);
});
