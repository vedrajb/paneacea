<script lang="ts">
  import XtermView from "./XtermView.svelte";
  import { paneHeader } from "../services/paneHeader";
  import { visiblePaneState } from "../services/paneStatus";
  import {
    RESUME_LOCK_MS,
    agentIsRunning,
    paneAgentResumable,
    paneAgentTitle,
  } from "../services/paneAgent";
  import { call } from "../services/backend";
  import agentIconUrl from "../../../icons/paneacea-agent.svg?url";
  import type { Layout, Pane, Settings } from "../services/backend";
  import type { Action } from "../shortcuts/actions";
  export let node: Layout;
  export let panes: Record<string, Pane>;
  export let activeId: string;
  export let settings: Settings;
  export let path: number[] = [];
  export let execute: (action: Action) => void;
  export let focus: (id: string) => void;
  export let resize: (path: number[], ratio: number) => void;
  export let error: (message: string) => void;
  let container: HTMLDivElement;
  let dragging = false;
  let ratio = node.ratio ?? 0.5;
  $: if (!dragging) ratio = node.ratio ?? 0.5;
  function start(event: PointerEvent) {
    dragging = true;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }
  function move(event: PointerEvent) {
    if (!dragging) return;
    const rect = container.getBoundingClientRect();
    ratio = Math.max(
      0.1,
      Math.min(
        0.9,
        node.orientation === "vertical"
          ? (event.clientX - rect.left) / rect.width
          : (event.clientY - rect.top) / rect.height,
      ),
    );
  }
  // Panes with an in-flight resume; the icon stays unclickable until the
  // resumed agent is seen running, the call fails, or RESUME_LOCK_MS passes.
  let resuming: Record<string, boolean> = {};
  function unlock(id: string) {
    if (!resuming[id]) return;
    const { [id]: _, ...rest } = resuming;
    resuming = rest;
  }
  $: for (const id of Object.keys(resuming)) {
    if (!panes[id] || panes[id].busy || agentIsRunning(panes[id].agent))
      unlock(id);
  }
  // Types the last agent's resume (session picker) command into the pane.
  async function resumeAgent(id: string) {
    if (resuming[id]) return;
    resuming = { ...resuming, [id]: true };
    focus(id);
    try {
      await call("agent.resume", { paneId: id });
    } catch (err) {
      unlock(id);
      error(err instanceof Error ? err.message : String(err));
      return;
    }
    setTimeout(() => unlock(id), RESUME_LOCK_MS);
  }
  function end() {
    if (!dragging) return;
    dragging = false;
    resize(path, ratio);
  }
</script>

{#if node.paneId && panes[node.paneId]}
  {@const pane = panes[node.paneId]}
  <section class:active={activeId === pane.id} class="pane" data-pane={pane.id}>
    <header class="pane-header">
      <button
        class="pane-name"
        on:click={() => focus(pane.id)}
        title={pane.currentWorkingDirectory}
        >{paneHeader(pane)}</button
      >
      {#if pane.agent}<button
          class="agent-icon-button"
          class:resuming={resuming[pane.id]}
          title={paneAgentTitle(pane.agent)}
          aria-label={paneAgentTitle(pane.agent)}
          aria-disabled={!paneAgentResumable(pane, resuming[pane.id])}
          aria-busy={!!resuming[pane.id]}
          on:click={() => {
            if (paneAgentResumable(pane, resuming[pane.id])) resumeAgent(pane.id);
            else focus(pane.id);
          }}
          ><img class="agent-icon" src={agentIconUrl} alt="" /></button
        >{/if}
      {#if visiblePaneState(pane.status)}<span class="pane-status"
          >{visiblePaneState(pane.status)}</span
        >{/if}
      <button
        title="Close pane"
        on:click={() => {
          focus(pane.id);
          execute("Terminal.ClosePane");
        }}>×</button
      >
    </header>
    {#if pane.error}<div class="pane-error">{pane.error}</div>{/if}
    {#key pane.id + ":" + pane.pid}<XtermView
        id={pane.id}
        active={activeId === pane.id}
        {settings}
        {execute}
        focus={() => focus(pane.id)}
        {error}
      />{/key}
  </section>
{:else if node.first && node.second}
  <div
    class="split"
    class:vertical={node.orientation === "vertical"}
    bind:this={container}
  >
    <div class="split-child" style:flex-basis={`${ratio * 100}%`}>
      <svelte:self
        node={node.first}
        {panes}
        {activeId}
        {settings}
        path={[...path, 0]}
        {execute}
        {focus}
        {resize}
        {error}
      />
    </div>
    <div
      class="splitter"
      role="slider"
      aria-label="Resize panes"
      aria-valuenow={Math.round(ratio * 100)}
      aria-valuemin="10"
      aria-valuemax="90"
      aria-orientation={node.orientation === "vertical"
        ? "vertical"
        : "horizontal"}
      tabindex="0"
      on:pointerdown={start}
      on:pointermove={move}
      on:pointerup={end}
      on:pointercancel={end}
      on:keydown={(event) => {
        if (
          ["ArrowLeft", "ArrowUp", "ArrowRight", "ArrowDown"].includes(
            event.key,
          )
        ) {
          event.preventDefault();
          resize(
            path,
            Math.min(
              0.9,
              Math.max(
                0.1,
                ratio +
                  (["ArrowLeft", "ArrowUp"].includes(event.key) ? -0.05 : 0.05),
              ),
            ),
          );
        }
      }}
    ></div>
    <div class="split-child grow">
      <svelte:self
        node={node.second}
        {panes}
        {activeId}
        {settings}
        path={[...path, 1]}
        {execute}
        {focus}
        {resize}
        {error}
      />
    </div>
  </div>
{/if}
