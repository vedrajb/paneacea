<script lang="ts">
  import { onMount } from "svelte";
  import { actions, defaults } from "../shortcuts/actions";

  export let overrides: Record<string, string>;
  export let close: () => void;
  let panel: HTMLDivElement;
  $: shortcuts = Object.entries({ ...defaults, ...overrides }).filter(
    ([, action]) =>
      action && Object.prototype.hasOwnProperty.call(actions, action),
  );

  onMount(() => {
    const previous = document.activeElement;
    panel.focus();
    return () => {
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus();
    };
  });
</script>

<div class="overlay">
  <div
    class="dialog shortcut-help"
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcut-heading"
    tabindex="-1"
    bind:this={panel}
  >
    <h2 id="shortcut-heading">Keyboard shortcuts</h2>
    <p>
      Current bindings, including your customizations. Ctrl+C copies selected
      text or interrupts when nothing is selected.
    </p>
    <table>
      <thead><tr><th>Shortcut</th><th>Action</th></tr></thead>
      <tbody>
        {#each shortcuts as [shortcut, action]}
          <tr
            ><td><kbd>{shortcut}</kbd></td><td
              >{actions[action as keyof typeof actions]}</td
            ></tr
          >
        {/each}
        <tr
          ><td><kbd>Ctrl+Mouse Wheel</kbd></td><td>Change terminal font size</td
          ></tr
        >
      </tbody>
    </table>
    <p>
      Ctrl+= / Ctrl+Shift+= use the main + key. Ctrl++ also accepts numpad +.
      Ctrl+Shift+/ is Ctrl+?.
    </p>
    <div class="dialog-actions">
      <button class="primary" on:click={close}>Close</button>
    </div>
  </div>
</div>

<style>
  .shortcut-help {
    width: 650px;
    max-height: calc(100vh - 80px);
    overflow: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    padding: 7px 9px;
    text-align: left;
    border-bottom: 1px solid var(--border);
  }
  td:first-child {
    white-space: nowrap;
  }
  p {
    font-size: 12px;
    color: var(--muted-foreground);
  }
</style>
