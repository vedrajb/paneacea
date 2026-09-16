<script lang="ts">
  import { onMount } from "svelte";
  import { Terminal } from "@xterm/xterm";
  import { FitAddon } from "@xterm/addon-fit";
  import { WebglAddon } from "@xterm/addon-webgl";
  import { bridge, bytes, call, type Settings } from "../services/backend";
  import { terminalCursorOptions } from "../services/terminalOptions";
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
    const terminalParameters = new URLSearchParams(window.location.search);
    let disposed = false,
      sequence = 0,
      queued = 0;
    let input = Promise.resolve();
    terminal = new Terminal({
      scrollback: settings.scrollback,
      fontSize: settings.fontSize,
      fontFamily: '"Cascadia Mono",Consolas,monospace',
      fontWeight: "400",
      fontWeightBold: "600",
      letterSpacing: 0,
      lineHeight: 1.0,
      customGlyphs: terminalParameters.get("customGlyphs") !== "false",
      ...terminalCursorOptions,
      theme: {
        background: "#1e1e1e",
        foreground: "#d4d4d4",
        selectionBackground: "#264f78",
      },
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host);
    let webgl: WebglAddon | undefined;
    let webglContextLoss: { dispose: () => void } | undefined;
    let webglUnavailable = false;
    function disposeWebgl() {
      webglContextLoss?.dispose();
      webglContextLoss = undefined;
      webgl?.dispose();
      webgl = undefined;
    }
    function updateRenderer() {
      const screen = terminal.element?.querySelector(".xterm-screen");
      if (!screen || !terminal.cols) return;
      const rect = screen.getBoundingClientRect();
      const cssCellWidth = rect.width / terminal.cols;
      const deviceCellWidth = cssCellWidth * window.devicePixelRatio;
      const renderer = terminalParameters.get("renderer") ?? "auto";
      const useWebgl = renderer !== "dom";
      if (useWebgl && !webgl && !webglUnavailable) {
        const addon = new WebglAddon();
        try {
          terminal.loadAddon(addon);
          webgl = addon;
          webglContextLoss = addon.onContextLoss(() => {
            disposeWebgl();
            webglUnavailable = true;
            host.dataset.renderer = "dom";
          });
        } catch {
          addon.dispose();
          webglUnavailable = true;
        }
      } else if (!useWebgl && webgl) {
        disposeWebgl();
      }
      host.dataset.renderer = webgl ? "webgl" : "dom";
      if (terminalParameters.has("terminalMetrics")) {
        console.table({
          renderer: host.dataset.renderer,
          dpr: window.devicePixelRatio,
          width: rect.width,
          height: rect.height,
          cols: terminal.cols,
          rows: terminal.rows,
          cssCellWidth,
          deviceCellWidth,
          cssCellHeight: rect.height / terminal.rows,
          deviceCellHeight:
            (rect.height / terminal.rows) * window.devicePixelRatio,
        });
      }
    }
    fit.fit();
    updateRenderer();
    const queryHandlers = [
      ...[
        { final: "n" },
        { prefix: "?", final: "n" },
        { final: "c" },
        { prefix: ">", final: "c" },
        { prefix: "=", final: "c" },
        { final: "t" },
        { intermediates: " ", final: "q" },
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
        updateRenderer();
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
    window.addEventListener("resize", resize);
    const focusTerminal = () => {
      if (!disposed && active && document.hasFocus()) terminal.focus();
    };
    host.addEventListener("paneacea-focus", focusTerminal);
    const wheel = (event: WheelEvent) => handleWheel(event, execute);
    host.addEventListener("wheel", wheel, { capture: true, passive: false });
    observer.observe(host);
    resize();
    if (active)
      requestAnimationFrame(() => requestAnimationFrame(focusTerminal));
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
          if (output.data) {
            await write(bytes(output.data));
            host.dataset.outputSequence = String(sequence);
          }
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
    return () => {
      disposed = true;
      clearTimeout(timer);
      fitTerminal = () => {};
      host.removeEventListener("paneacea-focus", focusTerminal);
      host.removeEventListener("wheel", wheel, true);
      window.removeEventListener("resize", resize);
      observer.disconnect();
      data.dispose();
      queryHandlers.forEach((handler) => handler.dispose());
      disposeWebgl();
      terminal.dispose();
      void bridge().Detach(streamID);
    };
  });
</script>

<div
  class="xterm-host"
  bind:this={host}
  on:focusin={() => {
    if (!active) focus();
  }}
></div>
