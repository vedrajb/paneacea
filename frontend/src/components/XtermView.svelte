<script lang="ts">
  import { onMount } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import { bridge, bytes, call, type Settings } from "../services/backend";
  import { handleKey, handleWheel, type Action } from "../shortcuts/actions";
  export let id: string;
  export let active: boolean;
  export let settings: Settings;
  export let execute: (action: Action) => void;
  export let focus: () => void;
  export let error: (message: string) => void;
  let host: HTMLDivElement;
  let terminal: Terminal;
  let fitTerminal = () => {};
  $: if (terminal && active) terminal.focus();
  $: if (terminal) {
    terminal.options.fontSize = settings.fontSize;
    terminal.options.scrollback = settings.scrollback;
    fitTerminal();
  }
  onMount(() => {
    const streamID = crypto.randomUUID();
    let disposed = false,
      sequence = 0,
      queued = 0;
    let input = Promise.resolve();
    terminal = new Terminal({
      scrollback: settings.scrollback,
      fontSize: settings.fontSize,
      fontFamily: '"Cascadia Mono",Consolas,monospace',
      cursorBlink: false,
      theme: {
        background: "#1e1e1e",
        foreground: "#d4d4d4",
        selectionBackground: "#264f78",
      },
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host);
    const queryHandlers = [
      ...[
        { final: "n" },
        { prefix: "?", final: "n" },
        { final: "c" },
        { prefix: ">", final: "c" },
        { prefix: "=", final: "c" },
        { final: "t" },
        { intermediates: "$", final: "p" },
        { prefix: "?", intermediates: "$", final: "p" },
      ].map((identifier) =>
        terminal.parser.registerCsiHandler(identifier, () => true),
      ),
      ...[10, 11, 12].map((identifier) =>
        terminal.parser.registerOscHandler(identifier, (data) => data === "?"),
      ),
    ];
    terminal.attachCustomKeyEventHandler((e) =>
      handleKey(
        e,
        terminal,
        execute,
        (text) => bridge().Copy(text),
        error,
        settings.keybindings,
      ),
    );
    const data = terminal.onData((text) => {
      if (queued + text.length > 65536) {
        error("Terminal input queue is full; wait before sending more input.");
        return;
      }
      queued += text.length;
      input = input
        .then(async () => {
          if (!disposed)
            await call("pane.sendInput", { paneId: id, data: text });
        })
        .catch((e) => error(String(e)))
        .finally(() => {
          queued -= text.length;
        });
    });
    let timer: ReturnType<typeof setTimeout>;
    let columns = 0,
      rows = 0;
    function resize() {
      clearTimeout(timer);
      timer = setTimeout(() => {
        if (disposed || host.clientWidth < 20 || host.clientHeight < 20) return;
        fit.fit();
        if (terminal.cols !== columns || terminal.rows !== rows) {
          columns = terminal.cols;
          rows = terminal.rows;
          void call("terminal.resize", { paneId: id, columns, rows }).catch(
            (e) => error(String(e)),
          );
        }
      }, 60);
    }
    const observer = new ResizeObserver(resize);
    fitTerminal = resize;
    const focusTerminal = () => terminal.focus();
    host.addEventListener("paneacea-focus", focusTerminal);
    const wheel = (event: WheelEvent) => handleWheel(event, execute);
    host.addEventListener("wheel", wheel, { capture: true, passive: false });
    observer.observe(host);
    resize();
    const write = (data: Uint8Array | string) =>
      new Promise<void>((resolve) => terminal.write(data, resolve));
    async function read() {
      try {
        while (!disposed) {
          const output = await bridge().ReadOutput(streamID, id, sequence);
          if (disposed) return;
          if (output.snapshot) {
            terminal.reset();
            if (output.columns && output.rows)
              terminal.resize(output.columns, output.rows);
          }
          sequence = output.sequence;
          if (output.data) await write(bytes(output.data));
          if (output.snapshot) resize();
          if (output.exited) {
            await write(
              "\r\n[Process exited. Use Terminal: Restart Exited Pane to relaunch.]\r\n",
            );
            return;
          }
        }
      } catch (e) {
        if (!disposed) error("Terminal disconnected: " + String(e));
      }
    }
    void read();
    if (active) terminal.focus();
    return () => {
      disposed = true;
      clearTimeout(timer);
      fitTerminal = () => {};
      host.removeEventListener("paneacea-focus", focusTerminal);
      host.removeEventListener("wheel", wheel, true);
      observer.disconnect();
      data.dispose();
      queryHandlers.forEach((handler) => handler.dispose());
      terminal.dispose();
      void bridge().Detach(streamID);
    };
  });
</script>

<div class="xterm-host" bind:this={host} on:focusin={focus}></div>
