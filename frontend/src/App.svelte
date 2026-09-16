<script lang="ts" context="module">
  function autofocus(node: HTMLInputElement) {
    const focus = () => {
      node.focus();
      node.select();
    };
    focus();
    requestAnimationFrame(focus);
  }
</script>

<script lang="ts">
  import { onMount, tick } from "svelte";
  import PaneTree from "./components/PaneTree.svelte";
  import ShortcutHelp from "./components/ShortcutHelp.svelte";
  import {
    bridge,
    call,
    type State,
    type Workspace,
    type Tab,
    type Profile,
    type Settings,
  } from "./services/backend";
  import {
    actions,
    resolve,
    shortcutsForAction,
    type Action,
  } from "./shortcuts/actions";
  import { neighbor, resizeTarget } from "./services/layout";
  import { tabLabel } from "./services/tabLabel";
  import appIconUrl from "../../icons/paneacea-app-icon.svg?url";
  import logoUrl from "../../icons/paneacea-logo-transparent.svg?url";
  import trayDarkUrl from "../../icons/paneacea-tray-dark.svg?url";
  import trayLightUrl from "../../icons/paneacea-tray-light.svg?url";
  let state: State | null = null;
  let workspace: Workspace | undefined;
  let tab: Tab | undefined;
  let connected = false,
    errorMessage = "",
    busy = false;
  let palette = false,
    query = "",
    settingsOpen = false,
    workspaceListOpen = false;
  let workspaceListIndex = 0;
  let profiles: Profile[] = [];
  let draft: Settings;
  let bindings = "";
  let modal: {
    title: string;
    value: string;
    label: string;
    submit: (value: string) => Promise<void>;
  } | null = null;
  let confirmation: { title: string; submit: () => Promise<void> } | null =
    null;
  let menu: {
    x: number;
    y: number;
    tabId?: string;
    workspaceId?: string;
  } | null = null;
  let draggedTab = "";
  let generation = 0;
  let shortcutsOpen = false;
  let fontChanges: Promise<void> = Promise.resolve();
  let startupFocusPending = true;
  let focusGeneration = 0;
  $: workspace = state?.workspaces.find(
    (w) => w.id === state?.activeWorkspaceId,
  );
  $: tab = workspace?.tabs.find((t) => t.id === workspace?.activeTabId);
  $: trayIconUrl =
    state?.settings.theme === "light" ? trayDarkUrl : trayLightUrl;
  let filtered: [Action, string][] = [];
  $: filtered = (Object.entries(actions) as [Action, string][]).filter(
    ([, label]) => label.toLowerCase().includes(query.toLowerCase()),
  );
  function error(message: string) {
    errorMessage = message;
  }
  function apply(next: State) {
    if (!state || next.revision > state.revision) state = next;
    connected = true;
  }
  function firstPaneId(): string | undefined {
    let node = tab?.rootLayoutNode;
    while (node && !node.paneId) node = node.first ?? node.second;
    return node?.paneId;
  }
  async function focusActiveTerminal(
    paneId = tab?.activePaneId,
  ): Promise<boolean> {
    await tick();
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => resolve()),
    );
    if (!paneId) return false;
    const host = document.querySelector<HTMLElement>(
      `[data-pane="${paneId}"] .xterm-host`,
    );
    const textarea = host?.querySelector<HTMLTextAreaElement>(
      ".xterm-helper-textarea",
    );
    if (!host || !textarea) return false;
    host.dispatchEvent(new Event("paneacea-focus"));
    textarea.focus({ preventScroll: true });
    return document.hasFocus() && document.activeElement === textarea;
  }
  async function focusFirstPaneOnStartup(): Promise<boolean> {
    const paneId = firstPaneId();
    if (!paneId) return false;
    if (tab?.activePaneId !== paneId) await mutate("pane.focus", { paneId });
    return focusActiveTerminal(paneId);
  }
  async function refresh() {
    try {
      const currentFocusGeneration = focusGeneration;
      const next = await call<State>("state.get");
      if (currentFocusGeneration === focusGeneration) apply(next);
      if (startupFocusPending && (await focusFirstPaneOnStartup()))
        startupFocusPending = false;
    } catch (e) {
      connected = false;
      error(String(e));
    }
  }
  async function mutate(method: string, params: unknown = {}) {
    busy = true;
    try {
      apply(await call(method, params));
      errorMessage = "";
    } finally {
      busy = false;
    }
  }
  function perform(method: string, params: unknown = {}) {
    void mutate(method, params).catch((e) => error(String(e)));
  }
  function focus(id: string) {
    focusGeneration++;
    if (tab) tab.activePaneId = id;
    perform("pane.focus", { paneId: id });
  }
  function textDialog(
    title: string,
    label: string,
    value: string,
    submit: (value: string) => Promise<void>,
  ) {
    modal = { title, label, value, submit };
  }
  function focusFirstMenuItem(node: HTMLElement) {
    requestAnimationFrame(() =>
      node.querySelector<HTMLButtonElement>("button")?.focus(),
    );
  }
  async function submitModal() {
    const pending = modal;
    if (!pending) return;
    try {
      await pending.submit(pending.value);
      if (modal === pending) modal = null;
    } catch (e) {
      error(String(e));
    }
  }
  async function submitConfirmation() {
    const pending = confirmation;
    if (!pending) return;
    try {
      await pending.submit();
      if (confirmation === pending) confirmation = null;
    } catch (e) {
      error(String(e));
    }
  }
  async function newWorkspace() {
    const root = await bridge().OpenFolder();
    if (!root) return;
    textDialog(
      "New workspace",
      "Name",
      root.split(/[/\\]/).filter(Boolean).pop() || "Workspace",
      async (name) => {
        await mutate("workspace.create", { name, rootDirectory: root });
      },
    );
  }
  async function openSettings() {
    if (!state) return;
    profiles = await call<Profile[]>("profiles.list");
    draft = JSON.parse(JSON.stringify(state.settings));
    bindings = JSON.stringify(draft.keybindings ?? {}, null, 2);
    settingsOpen = true;
  }
  function adjustTerminalFontSize(delta: number): Promise<void> {
    const change = fontChanges.then(async () => {
      if (!state) return;
      const fontSize = Math.max(
        8,
        Math.min(32, state.settings.fontSize + delta),
      );
      if (fontSize === state.settings.fontSize) return;
      await mutate("settings.set", {
        settings: { ...state.settings, fontSize },
      });
    });
    fontChanges = change.catch(() => {});
    return change;
  }
  async function executeAsync(action: Action) {
    palette = false;
    menu = null;
    const paneId = tab?.activePaneId;
    const pane = paneId ? state?.panes[paneId] : undefined;
    switch (action) {
      case "Help.ShowShortcuts":
        shortcutsOpen = true;
        return;
      case "Terminal.IncreaseFontSize":
        await adjustTerminalFontSize(1);
        return;
      case "Terminal.DecreaseFontSize":
        await adjustTerminalFontSize(-1);
        return;
      case "Terminal.Focus":
        settingsOpen = false;
        shortcutsOpen = false;
        await focusActiveTerminal();
        return;
      case "Paneacea.CommandPalette":
        query = "";
        palette = true;
        return;
      case "Paneacea.Settings":
        await openSettings();
        return;
      case "Paneacea.Reconnect":
        generation++;
        await refresh();
        return;
      case "Workspace.New":
        await newWorkspace();
        return;
      case "Workspace.OpenSelector":
        openWorkspaceSelector();
        return;
      case "Workspace.Next":
      case "Workspace.Previous":
        if (state && state.workspaces.length) {
          const index = state.workspaces.findIndex(
            (w) => w.id === workspace?.id,
          );
          const next =
            (index +
              (action === "Workspace.Next" ? 1 : -1) +
              state.workspaces.length) %
            state.workspaces.length;
          await mutate("workspace.switch", {
            workspaceId: state.workspaces[next].id,
          });
        }
        return;
    }
    if (!workspace) {
      if (action === "Terminal.NewTab") await newWorkspace();
      return;
    }
    switch (action) {
      case "Workspace.Rename":
        textDialog("Rename workspace", "Name", workspace.name, async (name) => {
          await mutate("workspace.rename", {
            workspaceId: workspace!.id,
            name,
          });
        });
        return;
      case "Workspace.ChangeRoot": {
        const root = await bridge().OpenFolder();
        if (root)
          await mutate("workspace.setRoot", {
            workspaceId: workspace.id,
            rootDirectory: root,
          });
        return;
      }
      case "Workspace.Close":
        confirmation = {
          title: `Close “${workspace.name}” and terminate all its terminal processes?`,
          submit: async () => {
            await mutate("workspace.close", { workspaceId: workspace!.id });
          },
        };
        return;
      case "Terminal.NewTab":
        await mutate("tab.create", { workspaceId: workspace.id });
        return;
      case "Terminal.NextTab":
      case "Terminal.PreviousTab":
        if (workspace.tabs.length) {
          const index = workspace.tabs.findIndex((t) => t.id === tab?.id);
          const next =
            (index +
              (action === "Terminal.NextTab" ? 1 : -1) +
              workspace.tabs.length) %
            workspace.tabs.length;
          await mutate("tab.focus", { tabId: workspace.tabs[next].id });
        }
        return;
    }
    if (!tab || !paneId) return;
    switch (action) {
      case "Terminal.CloseTab":
        await mutate("tab.close", { tabId: tab.id });
        return;
      case "Terminal.OpenTabRenamer": {
        const tabId = tab.id;
        const tabIndex = workspace.tabs.findIndex(
          (entry) => entry.id === tabId,
        );
        textDialog(
          "Rename tab",
          "Title",
          tabLabel(tab, tabIndex),
          async (title) => {
            await mutate("tab.rename", { tabId, title });
          },
        );
        return;
      }
      case "Terminal.ClosePane":
        await mutate("pane.close", { paneId });
        return;
      case "Terminal.Restart":
        await mutate("pane.restart", { paneId });
        return;
      case "Agent.Resume":
        await mutate("agent.resume", { paneId });
        return;
      case "Terminal.SplitPaneRight":
      case "Terminal.SplitPaneDown":
      case "Terminal.SplitPaneAuto": {
        let orientation = "vertical";
        if (action === "Terminal.SplitPaneDown") orientation = "horizontal";
        if (action === "Terminal.SplitPaneAuto") {
          const rect = document
            .querySelector(`[data-pane="${paneId}"]`)
            ?.getBoundingClientRect();
          if (rect && rect.height > rect.width) orientation = "horizontal";
        }
        await mutate("pane.split", { paneId, orientation });
        return;
      }
    }
    if (action.startsWith("Terminal.MoveFocus")) {
      const id = neighbor(
        tab.rootLayoutNode,
        paneId,
        action.replace("Terminal.MoveFocus", ""),
      );
      if (id) focus(id);
    }
    if (action.startsWith("Terminal.ResizePane")) {
      const resize = resizeTarget(
        tab.rootLayoutNode,
        paneId,
        action.replace("Terminal.ResizePane", ""),
      );
      if (resize) await mutate("pane.resize", { tabId: tab.id, ...resize });
    }
  }
  function execute(action: Action) {
    void executeAsync(action).catch((e) => error(String(e)));
  }
  function selectWorkspace(workspaceId: string) {
    workspaceListOpen = false;
    perform("workspace.switch", { workspaceId });
  }
  function openWorkspaceSelector() {
    const index = state?.workspaces.findIndex((item) => item.id === workspace?.id) ?? -1;
    workspaceListIndex = index >= 0 ? index : 0;
    workspaceListOpen = true;
  }
  function focusWorkspaceSelector(node: HTMLElement) {
    requestAnimationFrame(() =>
      node
        .querySelectorAll<HTMLButtonElement>("button")
        [workspaceListIndex]?.focus(),
    );
  }
  function workspaceSelectorKey(event: KeyboardEvent) {
    const workspaces = state?.workspaces ?? [];
    if (!workspaces.length) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      workspaceListIndex =
        (workspaceListIndex + direction + workspaces.length) % workspaces.length;
      const selector = event.currentTarget as HTMLElement;
      requestAnimationFrame(() =>
        selector
          .querySelectorAll<HTMLButtonElement>("button")
          [workspaceListIndex]?.focus(),
      );
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      selectWorkspace(workspaces[workspaceListIndex].id);
    }
  }
  function dismissPopups() {
    palette = false;
    settingsOpen = false;
    workspaceListOpen = false;
    modal = null;
    confirmation = null;
    menu = null;
    shortcutsOpen = false;
  }
  function popupKey(event: KeyboardEvent) {
    if (event.defaultPrevented) return;
    const popupOpen =
      palette ||
      modal ||
      confirmation ||
      settingsOpen ||
      workspaceListOpen ||
      shortcutsOpen ||
      menu;
    if (!popupOpen) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      dismissPopups();
      return;
    }
    if (event.key === "Enter" && !event.repeat && !event.isComposing) {
      const target = event.target;
      const isButton = target instanceof HTMLButtonElement;
      const isTextArea = target instanceof HTMLTextAreaElement;
      if (!isButton && !isTextArea) {
        if (confirmation) {
          event.preventDefault();
          event.stopPropagation();
          void submitConfirmation();
          return;
        }
        if (palette) {
          event.preventDefault();
          event.stopPropagation();
          if (filtered.length) execute(filtered[0][0]);
          return;
        }
        if (modal) {
          event.preventDefault();
          event.stopPropagation();
          void submitModal();
          return;
        }
        if (settingsOpen) {
          event.preventDefault();
          event.stopPropagation();
          void saveSettings();
          return;
        }
        if (shortcutsOpen) {
          event.preventDefault();
          event.stopPropagation();
          shortcutsOpen = false;
          return;
        }
      }
    }
    if (event.target instanceof Element && event.target.closest(".xterm")) {
      event.preventDefault();
      event.stopPropagation();
    }
  }
  function globalKey(event: KeyboardEvent) {
    if (event.defaultPrevented) return;
    const terminalTarget =
      event.target instanceof Element && event.target.closest(".xterm");
    const action = resolve(event, state?.settings.keybindings);
    if (terminalTarget) {
      if (!action) return;
      event.preventDefault();
      if (!event.repeat) execute(action);
      return;
    }

    if (event.key === "Escape") {
      event.preventDefault();
      dismissPopups();
      return;
    }
    if (action === "Terminal.Focus") {
      event.preventDefault();
      if (!event.repeat) execute(action);
      return;
    }
    if (
      palette ||
      modal ||
      confirmation ||
      settingsOpen ||
      workspaceListOpen ||
      shortcutsOpen ||
      menu
    )
      return;
    if (
      event.target instanceof HTMLInputElement ||
      event.target instanceof HTMLTextAreaElement ||
      event.target instanceof HTMLSelectElement
    )
      return;
    if (action) {
      event.preventDefault();
      if (!event.repeat) execute(action);
    }
  }
  async function saveSettings() {
    try {
      const parsed = JSON.parse(bindings);
      if (
        !parsed ||
        Array.isArray(parsed) ||
        typeof parsed !== "object" ||
        Object.values(parsed).some(
          (v) => typeof v !== "string" || (v !== "" && !(v in actions)),
        )
      )
        throw new Error(
          "Keybindings must map key combinations to action names, or an empty string to disable a binding.",
        );
      draft.keybindings = parsed;
      await mutate("settings.set", { settings: draft });
      settingsOpen = false;
      generation++;
    } catch (e) {
      error(String(e));
    }
  }
  onMount(() => {
    let favicon = document.querySelector<HTMLLinkElement>(
      'link[data-paneacea-favicon="true"]',
    );
    if (!favicon) {
      favicon = document.createElement("link");
      favicon.rel = "icon";
      favicon.dataset.paneaceaFavicon = "true";
      document.head.append(favicon);
    }
    favicon.type = "image/svg+xml";
    favicon.href = appIconUrl;

    function refocusTerminal() {
      if (!state || !connected || busy) return;
      if (startupFocusPending) {
        void focusFirstPaneOnStartup().then((focused) => {
          if (focused) startupFocusPending = false;
        });
      } else {
        void focusActiveTerminal();
      }
    }
    window.addEventListener("focus", refocusTerminal);

    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      if (stopped) return;
      if (!busy) await refresh();
      if (!stopped) timer = setTimeout(poll, 1500);
    }
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
      window.removeEventListener("focus", refocusTerminal);
    };
  });
</script>

<svelte:window on:keydown|capture={popupKey} on:keydown={globalKey} />
<div class="application" class:light={state?.settings.theme === "light"}>
  <header class="titlebar">
    <div class="brand"><img
        class="brand-icon"
        src={appIconUrl}
        alt=""
        aria-hidden="true"
      /> <span>Paneacea</span></div>
    <span
      class="window-title"
      title={workspace ? `${workspace.name} ${workspace.rootDirectory}` : undefined}
      >{#if workspace}<span class="window-workspace-name">{workspace.name}</span
      ><span class="window-workspace-path">{workspace.rootDirectory}</span
      >{:else}Persistent terminal workspaces{/if}</span
    ><button
      class="command-trigger"
      on:click={() => execute("Paneacea.CommandPalette")}
      >Search commands <kbd>Ctrl Shift P</kbd></button
    ><div class="titlebar-actions">
      <span
        class="runtime-status"
        class:offline={!connected}
        role="status"
        >{connected ? "● Runtime connected" : "○ Runtime disconnected"}</span>
    </div>
  </header>
  <div class="workbench">
    <aside class="sidebar" aria-label="Workspace controls">
      <div class="sidebar-top">
        <button
          class:active={workspaceListOpen}
          title="Workspaces"
          aria-label="Workspaces"
          aria-expanded={workspaceListOpen}
          on:click={openWorkspaceSelector}
          ><svg viewBox="0 0 24 24" aria-hidden="true"><path
              d="M4 5.5h6.5v5H4zM13.5 5.5H20v5h-6.5zM4 13.5h6.5v5H4zM13.5 13.5H20v5h-6.5z"
            /></svg></button
        ><button
          title="New workspace"
          aria-label="New workspace"
          on:click={() => execute("Workspace.New")}
          ><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg></button
        >
      </div>
      <div class="sidebar-spacer"></div>
      <button
        title="Settings"
        aria-label="Settings"
        on:click={() => execute("Paneacea.Settings")}
        ><svg viewBox="0 0 24 24" aria-hidden="true"><path
            d="M12 8.25a3.75 3.75 0 1 0 0 7.5 3.75 3.75 0 0 0 0-7.5Zm8 3.75 1.5-1-1.5-2.6-1.75.5a6.7 6.7 0 0 0-1.7-1L16.3 6h-3l-.75 1.9a6.7 6.7 0 0 0-1.7 1L9.1 8.4 7.6 11l1.5 1a6.2 6.2 0 0 0 0 2l-1.5 1 1.5 2.6 1.75-.5a6.7 6.7 0 0 0 1.7 1l.75 1.9h3l.75-1.9a6.7 6.7 0 0 0 1.7-1l1.75.5 1.5-2.6-1.5-1a6.2 6.2 0 0 0 0-2Z"
          /></svg></button
      >
    </aside>
    <main>
      <div class="tabs" role="tablist" aria-label="Terminal tabs">
        {#each workspace?.tabs ?? [] as item, index (item.id)}<button
            role="tab"
            aria-selected={tab?.id === item.id}
            class:active={tab?.id === item.id}
            draggable="true"
            on:dragstart={() => (draggedTab = item.id)}
            on:dragover={(e) => e.preventDefault()}
            on:drop={() => {
              if (draggedTab) perform("tab.move", { tabId: draggedTab, index });
              draggedTab = "";
            }}
            on:click={() => perform("tab.focus", { tabId: item.id })}
            on:dblclick={() => {
              const tabId = item.id;
              textDialog(
                "Rename tab",
                "Title",
                tabLabel(item, index),
                async (title) => {
                  await mutate("tab.rename", { tabId, title });
                },
              );
            }}
            on:contextmenu={(e) => {
              e.preventDefault();
              menu = {
                x: Math.min(e.clientX, window.innerWidth - 210),
                y: Math.min(e.clientY, window.innerHeight - 160),
                tabId: item.id,
              };
            }}
            ><img
              class="tab-icon"
              src={trayIconUrl}
              alt=""
              aria-hidden="true"
            /><span>{tabLabel(item, index)}</span
            >{#if Object.values(state?.panes ?? {}).some((p) => p.tabId === item.id && p.agent?.state === "waiting")}<span
                class="waiting">!</span
              >{/if}</button
          >{/each}<button
          class="new-tab"
          title="New terminal tab"
          on:click={() => execute("Terminal.NewTab")}>+</button
        >
        <div class="spacer"></div>
        <button
          title="Split right"
          on:click={() => execute("Terminal.SplitPaneRight")}>◫</button
        ><button
          title="Split down"
          on:click={() => execute("Terminal.SplitPaneDown")}>⬒</button
        >
      </div>
      {#if errorMessage}<div class="error-banner" role="alert">
          <span>{errorMessage}</span><button
            title="Dismiss error"
            on:click={() => (errorMessage = "")}>×</button
          >
        </div>{/if}
      <div class="terminal-area">
        {#if tab && state}{#key workspace?.id + ":" + tab.id + ":" + generation}<PaneTree
              node={tab.rootLayoutNode}
              panes={state.panes}
              activeId={tab.activePaneId}
              settings={state.settings}
              {execute}
              {focus}
              resize={(path, ratio) =>
                perform("pane.resize", { tabId: tab?.id, path, ratio })}
              {error}
            />{/key}{:else}<div class="welcome">
            <img class="welcome-mark" src={logoUrl} alt="Paneacea" />
            <h1>Your workspace, still running.</h1>
            <p>
              Organize shells and agents into workspaces.<br />Close the window
              and reconnect when you’re ready.
            </p>
            <button
              class="primary"
              on:click={() =>
                execute(workspace ? "Terminal.NewTab" : "Workspace.New")}
              >{workspace ? "Open a terminal" : "Create a workspace"}</button
            ><button on:click={() => execute("Paneacea.CommandPalette")}
              >Explore commands <kbd>Ctrl Shift P</kbd></button
            >
          </div>{/if}
      </div>
    </main>
  </div>
</div>
{#if workspaceListOpen}<div
    class="overlay workspace-selector-overlay"
    role="presentation"
    on:click={() => (workspaceListOpen = false)}
  >
    <div
      class="workspace-list"
      role="dialog"
      aria-label="Workspace selector"
      aria-modal="true"
      tabindex="-1"
      use:focusWorkspaceSelector
      on:click|stopPropagation
      on:keydown={workspaceSelectorKey}
    >
      <div class="workspace-list-title">Workspaces</div>
      {#each state?.workspaces ?? [] as item}<button
          class:active={item.id === workspace?.id}
          on:click={() => selectWorkspace(item.id)}
          ><span>{item.name}</span><small>{item.rootDirectory}</small></button
        >{/each}
    </div>
  </div>{/if}
{#if shortcutsOpen}
  <ShortcutHelp
    overrides={state?.settings.keybindings ?? {}}
    close={() => (shortcutsOpen = false)}
  />
{/if}
{#if palette}<div
    class="overlay"
    role="presentation"
    on:click={(e) => {
      if (e.target === e.currentTarget) palette = false;
    }}
  >
    <div
      class="palette"
      role="dialog"
      tabindex="-1"
      aria-label="Command palette"
      aria-modal="true"
    >
      <input
        aria-label="Search commands"
        placeholder="Type a command…"
        bind:value={query}
        use:autofocus
      />
      <div class="command-list">
        {#each filtered as [action, label]}
          <button on:click={() => execute(action)}>
            <span class="command-label">{label}</span>
            <span class="command-shortcuts">
              {#each shortcutsForAction(
                action,
                state?.settings.keybindings,
              ) as shortcut}
                <kbd>{shortcut}</kbd>
              {/each}
              <span class="command-enter">↵</span>
            </span>
          </button>
        {/each}
      </div>
    </div>
  </div>{/if}
{#if modal}<div class="overlay">
    <form
      class="dialog"
      on:submit|preventDefault={submitModal}
    >
      <h2>{modal.title}</h2>
      <label
        >{modal.label}<input
          bind:value={modal.value}
          required
          use:autofocus
        /></label
      >
      <div class="dialog-actions">
        <button type="button" on:click={() => (modal = null)}>Cancel</button
        ><button class="primary" disabled={busy}>Save</button>
      </div>
    </form>
  </div>{/if}
{#if confirmation}<div class="overlay">
    <div
      class="dialog"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label="Confirm close"
    >
      <h2>{confirmation.title}</h2>
      <p>This ends the workspace’s shells and agents.</p>
      <div class="dialog-actions">
        <button on:click={() => (confirmation = null)}>Cancel</button><button
          class="danger"
          on:click={submitConfirmation}>Close workspace</button
        >
      </div>
    </div>
  </div>{/if}
{#if settingsOpen && draft}<div class="overlay">
    <form class="dialog settings" on:submit|preventDefault={saveSettings}>
      <h2>Settings</h2>
      <label
        >Default shell<select
          value={draft.defaultShell.id}
          on:change={(e) => {
            const profile = profiles.find(
              (p) => p.id === e.currentTarget.value,
            );
            if (profile) draft.defaultShell = profile;
          }}
          >{#each profiles as profile}<option
              value={profile.id}
              disabled={!profile.available}
              >{profile.name}{profile.available ? "" : " (unavailable)"}</option
            >{/each}</select
        ></label
      ><label
        >Font size<input
          type="number"
          min="8"
          max="32"
          bind:value={draft.fontSize}
        /></label
      ><label
        >Scrollback lines<input
          type="number"
          min="0"
          max="100000"
          bind:value={draft.scrollback}
        /></label
      ><label
        >Appearance<select bind:value={draft.theme}
          ><option value="dark">Dark</option><option value="light"
            >Light chrome / dark terminal</option
          ></select
        ></label
      ><label
        >Custom keybindings<textarea
          rows="7"
          bind:value={bindings}
          spellcheck="false"
        ></textarea></label
      >
      <p class="hint">
        Map a combination such as “Ctrl+o” to “Terminal.NewTab”. An empty
        action disables a default. Full action names are listed below.
      </p>
      <details>
        <summary>Available actions</summary>
        <div class="action-reference">
          {#each Object.keys(actions) as action}<code>{action}</code>{/each}
        </div>
      </details>
      <div class="dialog-actions">
        <button type="button" on:click={() => (settingsOpen = false)}
          >Cancel</button
        ><button class="primary" disabled={busy}>Save settings</button>
      </div>
    </form>
  </div>{/if}
{#if menu}<div
    class="menu-dismiss"
    role="presentation"
    on:click={() => (menu = null)}
    on:contextmenu|preventDefault={() => (menu = null)}
  ></div>
  <div
    class="context-menu"
    use:focusFirstMenuItem
    style:left={`${menu.x}px`}
    style:top={`${menu.y}px`}
  >
    {#if menu.tabId}<button
        on:click={() => {
          if (!menu?.tabId) return;
          const tabId = menu.tabId;
          const entry = workspace?.tabs.find((t) => t.id === tabId);
          const entryIndex =
            workspace?.tabs.findIndex((t) => t.id === tabId) ?? 0;
          menu = null;
          textDialog(
            "Rename tab",
            "Title",
            entry ? tabLabel(entry, entryIndex) : "",
            async (title) => {
              await mutate("tab.rename", { tabId, title });
            },
          );
        }}>Rename tab</button
      ><button
        on:click={() => {
          if (!menu?.tabId) return;
          perform("tab.close", { tabId: menu.tabId });
          menu = null;
        }}>Close tab and its panes</button
      >{/if}
  </div>{/if}
