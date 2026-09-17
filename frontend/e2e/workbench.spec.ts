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
    const focusRace: any = { paneFocusReleases: [] };
    if (window.location.search.includes("startupFirstPane")) {
      const tab = state.workspaces[0].tabs[0];
      state.panes["p-second"] = {
        ...state.panes.p,
        id: "p-second",
        pid: 124,
      };
      tab.activePaneId = "p-second";
      tab.rootLayoutNode = {
        orientation: "vertical",
        ratio: 0.5,
        first: { paneId: "p" },
        second: { paneId: "p-second" },
      };
    }
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
    (window as any).testFocusRace = focusRace;
    (window as any).runtime = {
      WindowMinimise: () => calls.push({ method: "window.minimise" }),
      WindowToggleMaximise: () => calls.push({ method: "window.maximise" }),
      Quit: () => calls.push({ method: "window.close" }),
    };
    const streams = new Set<string>();
    window.go = {
      desktop: {
        App: {
          Call: async (method: string, params: any) => {
            calls.push({ method, params });
            if (
              method === "state.get" &&
              window.location.search.includes("runtimeDisconnected")
            )
              throw new Error("runtime unavailable");
            if (
              method === "pane.focus" &&
              window.location.search.includes("pipeClosed")
            )
              throw new Error("The pipe is being closed.");
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
            const result = structuredClone(state);
            if (method === "state.get" && focusRace.holdStateGet) {
              focusRace.stateGetWaiting = true;
              await new Promise<void>((resolve) => {
                focusRace.releaseStateGet = resolve;
              });
            }
            if (method === "pane.focus" && focusRace.holdPaneFocus) {
              focusRace.paneFocusWaiting = true;
              await new Promise<void>((resolve) => {
                focusRace.paneFocusReleases.push(resolve);
              });
            }
            return result;
          },
          ReadOutput: async (
            streamID: string,
            id: string,
            sequence: number,
          ) => {
            if (window.location.search.includes("terminalDisconnected"))
              throw new Error("runtime unavailable");
            if (!streams.has(streamID)) {
              streams.add(streamID);
              calls.push({ method: "terminal.output", id });
              const modelStart = " model:      ";
              const modelValue = "gpt-5.6-luna max";
              const modelGap = "   ";
              const modelHint = "/model to change";
              const modelHintOutput =
                "\x1b[38;5;6m\x1b[22m/model\x1b[m\x1b[2m to change";
              const titleStart = " >_ ";
              const titleName = "OpenAI Codex";
              const titleVersion = " (v0.154.0)";
              const directoryStart = " directory: ";
              const directoryValue = "~\\workspace\\ai\\paneacea";
              const modelPadding = " ".repeat(
                68 -
                  modelStart.length -
                  modelValue.length -
                  modelGap.length -
                  modelHint.length,
              );
              const panel = [
                `\x1b[2m╭${"─".repeat(68)}╮\x1b[22m`,
                `\x1b[2m│${titleStart}\x1b[22m\x1b[1m${titleName}\x1b[22m\x1b[2m${titleVersion}${" ".repeat(68 - titleStart.length - titleName.length - titleVersion.length)}│\x1b[22m`,
                `\x1b[2m│${"".padEnd(68)}│\x1b[22m`,
                `\x1b[2m│${modelStart}\x1b[22m${modelValue}\x1b[2m${modelGap}\x1b[22m${modelHintOutput}${modelPadding}│\x1b[2m`,
                `\x1b[2m│${directoryStart}\x1b[22m${directoryValue}\x1b[2m${" ".repeat(68 - directoryStart.length - directoryValue.length)}│\x1b[22m`,
                `\x1b[2m╰${"─".repeat(68)}╯\x1b[22m`,
              ].join("\r\n");
              return {
                data: btoa(
                  String.fromCharCode(
                    ...new TextEncoder().encode(
                      `${panel}\x1b[0m\r\n\x1b[32mPaneacea runtime connected\x1b[0m\r\nPS C:\\dev\\paneacea> `,
                    ),
                  ),
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

test("shows runtime connection icons", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTitle("Runtime connected")).toHaveAttribute(
    "src",
    /%2301A601/,
  );

  await page.goto("/?runtimeDisconnected");
  await expect(page.getByTitle("Runtime disconnected")).toHaveAttribute(
    "src",
    /%23FC4032/,
  );
});

test("uses an icon-only title-bar brand", async ({ page }) => {
  await page.goto("/");
  const brand = page.locator(".brand");
  await expect(brand).toHaveText("");
  await expect(brand.getByTitle("Paneacea")).toBeVisible();
});

test("uses the workspace header as draggable window chrome", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator(".titlebar")).toHaveCSS(
    "--wails-draggable",
    "drag",
  );
  await page.getByTitle("Minimise").click();
  await page.getByTitle("Maximise or restore").click();
  await page.locator(".window-title").dblclick();
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls
          .filter((entry: any) => entry.method.startsWith("window."))
          .map((entry: any) => entry.method),
      ),
    )
    .toEqual([
      "window.minimise",
      "window.maximise",
      "window.maximise",
      "window.close",
    ]);
});

test("shows an auto-dismissing runtime restart toast", async ({ page }) => {
  await page.goto("/?startupFirstPane&pipeClosed");
  await page.locator('[data-pane="p"] .xterm-helper-textarea').focus();
  const toast = page.getByText("Restarting runtime...");
  await expect(toast).toBeVisible();
  await expect(toast).toHaveCSS("background-color", "rgb(37, 37, 38)");
  await expect(toast).toHaveCSS("border-top-left-radius", "6px");
  await expect(toast).toHaveCSS("font-size", "14px");
  const toastBox = await toast.boundingBox();
  const viewportHeight = await page.evaluate(() => window.innerHeight);
  expect(toastBox).not.toBeNull();
  expect(toastBox!.y).toBeGreaterThan(viewportHeight / 2);
  await expect(
    page.getByText("The pipe is being closed."),
  ).toHaveCount(0);
  await expect(page.getByText("Restarting runtime...")).toHaveCount(0, {
    timeout: 4000,
  });
});

test("handles named-pipe shutdown as a runtime disconnection", async ({
  page,
}) => {
  await page.goto("/?startupFirstPane&pipeClosed");
  await page.locator('[data-pane="p"] .xterm-helper-textarea').focus();
  await expect(
    page.getByText("Restarting runtime..."),
  ).toBeVisible();
  await expect(page.getByText("The pipe is being closed.")).toHaveCount(0);
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

test("renders the Codex startup border without dashed cell gaps", async ({
  page,
}) => {
  await page.goto("/?renderer=auto&customGlyphs=true");
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-output-sequence",
    "100",
  );
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-renderer",
    "webgl",
  );
  const screen = await page.locator(".xterm-screen").boundingBox();
  expect(screen).not.toBeNull();
  await expect(page).toHaveScreenshot("codex-startup-border.png", {
    clip: {
      x: Math.floor(screen!.x),
      y: Math.floor(screen!.y),
      width: Math.min(Math.floor(screen!.width), 640),
      height: Math.min(Math.floor(screen!.height), 125),
    },
    maxDiffPixelRatio: 0.001,
  });
});

test.describe("high-DPI terminal rendering", () => {
  test.use({ deviceScaleFactor: 2 });

  test("keeps WebGL selected at 200% and captures the DOM comparison", async ({
    page,
  }) => {
    await page.goto("/?renderer=auto&terminalMetrics");
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-output-sequence",
      "100",
    );
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-renderer",
      "webgl",
    );
    await expect
      .poll(() =>
        page.evaluate(() =>
          (window as any).testCalls.some(
            (entry: any) => entry.method === "terminal.resize",
          ),
        ),
      )
      .toBe(true);
    await page.goto("/?renderer=dom");
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-output-sequence",
      "100",
    );
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-renderer",
      "dom",
    );
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(page.locator(".xterm-screen")).toHaveScreenshot(
      "codex-startup-border-200-percent.png",
      { maxDiffPixelRatio: 0.001 },
    );
  });
});

test("compares WebGL font glyphs on the Codex startup border", async ({
  page,
}) => {
  await page.goto("/?renderer=webgl&customGlyphs=false");
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-output-sequence",
    "100",
  );
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-renderer",
    "webgl",
  );
  const screen = await page.locator(".xterm-screen").boundingBox();
  expect(screen).not.toBeNull();
  await expect(page).toHaveScreenshot("codex-startup-border-webgl-font.png", {
    clip: {
      x: Math.floor(screen!.x),
      y: Math.floor(screen!.y),
      width: Math.min(Math.floor(screen!.width), 640),
      height: Math.min(Math.floor(screen!.height), 125),
    },
    maxDiffPixelRatio: 0.001,
  });
});

test("compares the DOM renderer on the Codex startup border", async ({
  page,
}) => {
  await page.goto("/?renderer=dom");
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-output-sequence",
    "100",
  );
  await expect(page.locator(".xterm-host")).toHaveAttribute(
    "data-renderer",
    "dom",
  );
  const screen = await page.locator(".xterm-screen").boundingBox();
  expect(screen).not.toBeNull();
  await expect(page).toHaveScreenshot("codex-startup-border-dom.png", {
    clip: {
      x: Math.floor(screen!.x),
      y: Math.floor(screen!.y),
      width: Math.min(Math.floor(screen!.width), 640),
      height: Math.min(Math.floor(screen!.height), 125),
    },
    maxDiffPixelRatio: 0.001,
  });
});

test.describe("fractional-DPI terminal rendering", () => {
  test.use({ deviceScaleFactor: 1.25 });

  test("keeps WebGL selected at 125% and captures the DOM comparison", async ({
    page,
  }) => {
    await page.goto("/?renderer=auto&terminalMetrics");
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-output-sequence",
      "100",
    );
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-renderer",
      "webgl",
    );
    await expect
      .poll(() =>
        page.evaluate(() =>
          (window as any).testCalls.some(
            (entry: any) => entry.method === "terminal.resize",
          ),
        ),
      )
      .toBe(true);
    await page.goto("/?renderer=dom");
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-output-sequence",
      "100",
    );
    await expect(page.locator(".xterm-host")).toHaveAttribute(
      "data-renderer",
      "dom",
    );
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(page.locator(".xterm-screen")).toHaveScreenshot(
      "codex-startup-border-auto-125-percent.png",
      { maxDiffPixelRatio: 0.001 },
    );
  });
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

test("focuses the selected tab's first pane after startup", async ({ page }) => {
  await page.goto("/?startupFirstPane");
  const firstTerminal = page.locator(
    '[data-pane="p"] .xterm-helper-textarea',
  );
  await expect(firstTerminal).toBeFocused();
  await page.getByTitle("Settings", { exact: true }).focus();
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect(firstTerminal).toBeFocused();
  await page.getByTitle("Settings", { exact: true }).focus();
  await page.keyboard.press("Control+`");
  await expect(firstTerminal).toBeFocused();
});

test("keeps the clicked pane active when a stale refresh arrives", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByTitle("Split right", { exact: true }).click();
  const panes = page.locator(".pane");
  await expect(panes).toHaveCount(2);
  await page.evaluate(() => {
    (window as any).testFocusRace.holdStateGet = true;
    (window as any).testFocusRace.holdPaneFocus = true;
  });
  await expect
    .poll(() => page.evaluate(() => (window as any).testFocusRace.stateGetWaiting))
    .toBe(true);
  await panes.first().locator(".xterm-helper-textarea").focus();
  await expect(panes.first()).toHaveClass(/active/);
  await expect
    .poll(() => page.evaluate(() => (window as any).testFocusRace.paneFocusWaiting))
    .toBe(true);
  await page.evaluate(() => {
    const focusRace = (window as any).testFocusRace;
    focusRace.holdStateGet = false;
    focusRace.releaseStateGet();
  });
  await expect(panes.first()).toHaveClass(/active/);
  await page.evaluate(() => {
    const focusRace = (window as any).testFocusRace;
    focusRace.holdPaneFocus = false;
    focusRace.paneFocusReleases.splice(0).forEach((resolve: () => void) =>
      resolve(),
    );
  });
});

test("matches the terminal viewport to the terminal background", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".xterm-viewport")).toHaveCSS(
    "background-color",
    "rgb(12, 12, 12)",
  );
});

test("uses a compact rounded terminal scrollbar", async ({ page }) => {
  await page.goto("/");
  const slider = page.locator(
    ".xterm-scrollable-element > .scrollbar.vertical > .slider",
  );
  await expect(slider).toHaveCSS("width", "4px");
  await expect(slider).toHaveCSS("border-top-left-radius", "3px");
});

test("uses a collapsed workspace rail with bottom-pinned Settings", async ({
  page,
}) => {
  await page.goto("/");

  const sidebar = page.getByRole("complementary", {
    name: "Workspace controls",
  });
  await expect(sidebar).toBeVisible();
  await expect(page.getByTitle("Workspaces", { exact: true })).toBeVisible();
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+b");
  const selector = page.getByRole("dialog", { name: "Workspace selector" });
  await expect(selector).toBeVisible();
  const selectorBox = await selector.boundingBox();
  const viewportWidth = await page.evaluate(() => window.innerWidth);
  expect(selectorBox).not.toBeNull();
  expect(
    Math.abs(selectorBox!.x + selectorBox!.width / 2 - viewportWidth / 2),
  ).toBeLessThanOrEqual(1);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(selector).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls.some(
          (entry: any) =>
            entry.method === "workspace.switch" &&
            entry.params.workspaceId === "other",
        ),
      ),
    )
    .toBe(true);

  await page.getByTitle("Workspaces", { exact: true }).click();
  await expect(selector).toBeVisible();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("Escape");
  await expect(selector).toHaveCount(0);
  expect(
    await page.evaluate(
      () =>
        (window as any).testCalls.filter(
          (entry: any) => entry.method === "workspace.switch",
        ).length,
    ),
  ).toBe(1);

  const sidebarBox = await sidebar.boundingBox();
  const settings = await page
    .getByTitle("Settings", { exact: true })
    .boundingBox();
  expect(sidebarBox).not.toBeNull();
  expect(settings).not.toBeNull();
  expect(settings!.y + settings!.height).toBeGreaterThanOrEqual(
    sidebarBox!.y + sidebarBox!.height - 8,
  );
});

test("renames the active workspace with Ctrl+Alt+R", async ({ page }) => {
  await page.goto("/");
  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.press("Control+Alt+r");
  await expect(
    page.getByRole("heading", { name: "Rename workspace", exact: true }),
  ).toBeVisible();
});

test("passes unbound Tab to the focused terminal", async ({ page }) => {
  await page.goto("/");
  const terminalInput = page.locator(".xterm-helper-textarea");
  await terminalInput.focus();
  await page.keyboard.press("Tab");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls
          .filter((entry: any) => entry.method === "pane.sendInput")
          .map((entry: any) => entry.params.data)
          .join(""),
      ),
    )
    .toContain("\t");
  await expect(terminalInput).toBeFocused();
});

test("copies terminal selections with Ctrl+C and pastes with Ctrl+V", async ({
  page,
}) => {
  await page.goto("/");
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  const terminalInput = page.locator(".xterm-helper-textarea");
  const screen = page.locator(".xterm-screen");
  await terminalInput.focus();
  const box = await screen.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + 5, box!.y + 5);
  await page.mouse.down();
  await page.mouse.move(box!.x + 120, box!.y + 5);
  await page.mouse.up();
  await page.keyboard.press("Control+c");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls.some(
          (entry: any) => entry.method === "copy",
        ),
      ),
    )
    .toBe(true);
  await page.evaluate(() => navigator.clipboard.writeText("pasted text"));
  await page.keyboard.press("Control+v");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (window as any).testCalls
          .filter((entry: any) => entry.method === "pane.sendInput")
          .map((entry: any) => entry.params.data)
          .join(""),
      ),
    )
    .toContain("pasted text");
});

test("font shortcuts and Ctrl+wheel resize terminals without remounting", async ({
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
  for (const shortcut of [
    "Control+Shift+Equal",
    "Control+NumpadAdd",
    "Control+Shift+NumpadAdd",
    "Control+Shift+Minus",
  ]) {
    await page.keyboard.press(shortcut);
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as any).testCalls.filter(
              (entry: any) => entry.method === "settings.set",
            ).length,
        ),
      )
      .toBe(1);
  }
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
    .toBe(15);
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
    .toBe(14);
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
    help.getByText("Ctrl+Alt+ArrowLeft", { exact: true }),
  ).toBeVisible();
  await expect(
    help.getByText("Ctrl+Alt+ArrowRight", { exact: true }),
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
