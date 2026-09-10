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
  import { countPanesForTab } from "./services/tabPaneCount";
  import {
    workspaceMenuItems,
    type WorkspaceMenuAction,
  } from "./services/workspaceMenu";
  import appIconUrl from "../../icons/paneacea-app-icon.svg?url";
  import logoUrl from "../../icons/paneacea-logo-transparent.svg?url";
  import trayDarkUrl from "../../icons/paneacea-tray-dark.svg?url";
  import trayLightUrl from "../../icons/paneacea-tray-light.svg?url";
  let state: State | null = null;
  let workspace: Workspace | undefined;
  let tab: Tab | undefined;
  let connected = false,
    errorMessage = "",
    sidebar = true,
    busy = false;
  let palette = false,
    query = "",
    settingsOpen = false;
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
    if (!state || next.revision >= state.revision) state = next;
    connected = true;
  }
  async function focusActiveTerminal(): Promise<boolean> {
    await tick();
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => resolve()),
    );
    const paneId = tab?.activePaneId;
    if (!paneId) return false;
    const host = document.querySelector<HTMLElement>(
      `[data-pane="${paneId}"] .xterm-host`,
    );
    if (host) {
      host.dispatchEvent(new Event("paneacea-focus"));
      host
        .querySelector<HTMLTextAreaElement>(".xterm-helper-textarea")
        ?.focus({ preventScroll: true });
      return true;
    }
    const textarea = document.querySelector<HTMLTextAreaElement>(
      `[data-pane="${paneId}"] .xterm-helper-textarea`,
    );
    if (!textarea) return false;
    textarea.focus({ preventScroll: true });
    return true;
  }
  async function refresh() {
    try {
      apply(await call("state.get"));
      if (startupFocusPending && (await focusActiveTerminal()))
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
  function runWorkspaceMenuAction(
    action: WorkspaceMenuAction,
    workspaceId: string,
  ) {
    const target = state?.workspaces.find((item) => item.id === workspaceId);
    if (!target) return;
    if (action === "Workspace.Rename") {
      textDialog("Rename workspace", "Name", target.name, async (name) => {
        await mutate("workspace.rename", { workspaceId, name });
      });
      return;
    }
    confirmation = {
      title: `Close “${target.name}” and terminate all its terminal processes?`,
      submit: async () => {
        await mutate("workspace.close", { workspaceId });
      },
    };
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
      case "Workspace.Switch":
        sidebar = true;
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
  function dismissPopups() {
    palette = false;
    settingsOpen = false;
    modal = null;
    confirmation = null;
    menu = null;
    shortcutsOpen = false;
  }
  function popupKey(event: KeyboardEvent) {
    if (event.defaultPrevented) return;
    const popupOpen =
      palette || modal || confirmation || settingsOpen || shortcutsOpen || menu;
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
    if (event.key === "Escape") {
      event.preventDefault();
      dismissPopups();
      return;
    }
    if (palette || modal || confirmation || settingsOpen || shortcutsOpen || menu)
      return;
    const action = resolve(event, state?.settings.keybindings);
    if (action === "Terminal.Focus") {
      event.preventDefault();
      if (!event.repeat) execute(action);
      return;
    }
    if (event.target instanceof Element && event.target.closest(".xterm"))
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
    >
  </header>
  <div class="workbench">
    <nav class="activity" aria-label="Activity bar">
      <button
        class:selected={sidebar}
        title="Explorer"
        on:click={() => (sidebar = !sidebar)}>▤</button
      ><button title="New workspace" on:click={() => execute("Workspace.New")}
        >⊞</button
      ><button
        title="Command palette"
        on:click={() => execute("Paneacea.CommandPalette")}>⌘</button
      >
      <div class="spacer"></div>
      <button title="Settings" on:click={() => execute("Paneacea.Settings")}
        >⚙</button
      >
    </nav>
    {#if sidebar}<aside class="explorer">
        <div class="section-heading">
          <span>EXPLORER</span><button
            title="New workspace"
            on:click={() => execute("Workspace.New")}>+</button
          >
        </div>
        {#each state?.workspaces ?? [] as item (item.id)}<div
            class="workspace-item"
          >
            <button
              class="workspace-label"
              class:selected={workspace?.id === item.id}
              title={item.rootDirectory}
              on:click={() =>
                perform("workspace.switch", { workspaceId: item.id })}
              on:contextmenu={(event) => {
                event.preventDefault();
                menu = {
                  x: Math.min(event.clientX, window.innerWidth - 210),
                  y: Math.min(event.clientY, window.innerHeight - 110),
                  workspaceId: item.id,
                };
              }}
              ><span class="disclosure"
                >{workspace?.id === item.id ? "⌄" : "›"}</span
              ><span class="workspace-name">{item.name}</span><span
                class="workspace-root">{item.rootDirectory}</span><small
                class="workspace-count">{item.tabs.length}</small
              ></button
            >
            {#if workspace?.id === item.id}<div class="workspace-contents">
                {#each item.tabs as entry, index (entry.id)}<button
                    class="tree-tab"
                    class:selected={tab?.id === entry.id}
                    on:click={() => perform("tab.focus", { tabId: entry.id })}
                    title={`${countPanesForTab(state?.panes ?? {}, entry.id)} panes`}
                    ><img
                      class="tree-icon"
                      src={trayIconUrl}
                      alt=""
                      aria-hidden="true"
                    /><span class="tree-name"
                      >{tabLabel(entry, index)}</span
                    ><small class="tab-count"
                      >{countPanesForTab(state?.panes ?? {}, entry.id)}</small
                    ></button
                  >{/each}
              </div>{/if}
          </div>{/each}
      </aside>{/if}
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
  <footer class="statusbar">
    <span class:offline={!connected}
      >{connected ? "● Runtime connected" : "○ Runtime disconnected"}</span
    ><span>{workspace?.name ?? "No workspace"}</span><span class="status-root"
      >{workspace?.rootDirectory ?? ""}</span
    >
    <div class="spacer"></div>
    {#if tab && state}<span
        >{state.panes[tab.activePaneId]?.executable.split(/[/\\]/).pop()}</span
      ><span
        >{state.panes[tab.activePaneId]?.agent?.state ??
          state.panes[tab.activePaneId]?.status}</span
      ><span>Pane {tab.activePaneId.slice(0, 6)}</span>{/if}
  </footer>
</div>
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
        Map a combination such as “Ctrl+Shift+t” to “Terminal.NewTab”. An empty
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
    {#if menu.workspaceId}{#each workspaceMenuItems as item}<button
          on:click={() => {
            if (!menu?.workspaceId) return;
            const workspaceId = menu.workspaceId;
            menu = null;
            runWorkspaceMenuAction(item.action, workspaceId);
          }}>{item.label}</button
        >{/each}{:else if menu.tabId}<button
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
