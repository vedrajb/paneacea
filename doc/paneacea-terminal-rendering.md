# Paneacea terminal rendering implementation

This document records the rendering changes that produced the current Paneacea terminal appearance. It summarizes the implementation and validation decisions from the rendering investigation so the current behavior can be maintained as the terminal evolves.

## Result

Paneacea keeps xterm.js as its terminal renderer and uses the WebGL addon with custom glyphs by default. This gives Codex and other terminal UIs continuous box-drawing strokes at the normal Paneacea display sizes. The DOM renderer remains available as an explicit comparison mode.

The terminal also starts child shells with color-capable terminal variables. This is required when Paneacea is launched from an environment such as VS Code that supplies `TERM=dumb` and `NO_COLOR=1`.

## Renderer and font configuration

The frontend uses these compatible package versions:

| Package | Version |
| --- | --- |
| `@xterm/xterm` | `6.0.0` |
| `@xterm/addon-fit` | `0.11.0` |
| `@xterm/addon-webgl` | `0.19.0` |

The terminal is initialized with:

```ts
fontFamily: '"Cascadia Mono",Consolas,monospace'
fontWeight: "400"
fontWeightBold: "600"
letterSpacing: 0
lineHeight: 1.0
customGlyphs: true
```

The baseline theme uses a dark `#1e1e1e` background, `#d4d4d4` foreground, and `#264f78` selection background. ANSI colors remain enabled so applications retain their own palette and styling.

## Renderer selection

`XtermView.svelte` selects the renderer from the page query string:

| Query | Behavior |
| --- | --- |
| no `renderer` parameter | WebGL with custom glyphs |
| `renderer=auto` | WebGL with custom glyphs |
| `renderer=webgl` | WebGL with custom glyphs |
| `renderer=dom` | xterm's default DOM renderer |

The first implementation selected DOM when the measured physical cell width was fractional. On the target Windows/WebView2 setup, that heuristic made the box border worse by switching to separately font-rendered `─` glyphs. The current policy therefore keeps WebGL for the default and uses DOM only when it is requested for diagnostics or comparison.

The WebGL addon is loaded after the terminal is opened. It is disposed when the pane is destroyed. If the WebGL context is lost or the addon cannot initialize, the pane records a DOM fallback for the remainder of that lifetime.

For diagnostics, add `terminalMetrics` to the URL. Paneacea then logs:

- device pixel ratio;
- terminal CSS dimensions;
- columns and rows;
- CSS and physical cell width and height; and
- the active renderer.

The active renderer is also exposed as `data-renderer` on `.xterm-host`. The output sequence is exposed as `data-output-sequence`, which lets browser tests wait until the first terminal snapshot has been rendered.

## Fit and resize behavior

`FitAddon` is loaded for each terminal. A `ResizeObserver` and the window resize event schedule a 60 ms debounced fit. After fitting, Paneacea:

1. reevaluates the renderer;
2. compares the current terminal dimensions with the previous dimensions; and
3. sends `terminal.resize` to the Go runtime only when the dimensions changed.

This keeps the xterm surface and the ConPTY dimensions in sync while avoiding repeated resize calls during a layout update.

## Preserving Codex colors

Codex chooses whether to emit colors from the child process environment. The Go launch path now supplies safe defaults when the pane has no meaningful terminal settings:

```text
TERM=xterm-256color
COLORTERM=truecolor
```

Existing non-empty values are preserved. Empty values and `TERM=dumb` are replaced with the color-capable defaults.

The ConPTY environment builder omits an inherited `NO_COLOR` variable unless the pane explicitly supplied `NO_COLOR`. This prevents a parent GUI environment from silently disabling colors for every child shell while still allowing an intentional per-pane override.

With these variables, Codex emits its colored model link, for example:

```text
ESC[38;5;6m ESC[22m /model ESC[m ESC[2m to change
```

Paneacea passes the ANSI stream through xterm.js; it does not replace Codex colors with frontend text replacements. Existing shells must be restarted to inherit the new environment.

## Codex border behavior

The Codex startup panel uses Unicode box-drawing characters. The important rendering path is:

```text
Codex ANSI output
    ↓
Go ConPTY capture and VT snapshot
    ↓
xterm.js buffer
    ↓
WebGL custom glyph atlas
```

The final bright right-edge cell seen during the monochrome investigation was caused by Codex resetting dim mode before `/model` and not restoring it before that row's closing border. Once Codex is allowed to emit its color sequence, it restores dim mode after the colored `/model` token and the closing border receives the intended dim styling. No byte-level border rewrite is needed.

## Validation

The browser fixture reproduces the Codex startup panel, including its ANSI styles and colored `/model` token. The visual matrix covers:

- default WebGL with custom glyphs;
- WebGL with custom glyphs disabled;
- explicit DOM rendering;
- automatic renderer selection at 125% device scale; and
- automatic renderer selection at 200% device scale.

The high-DPI tests first verify that the automatic policy selects WebGL, then capture an explicit DOM comparison because headless Edge does not reliably include the WebGL canvas in screenshots at those device scale factors.

Current checks:

```powershell
cd frontend
npm run check
npm test -- --run
npx playwright test e2e/workbench.spec.ts --grep "Codex startup|200% and|125% and"

cd ..
go test ./...
```

The frontend check, 102 frontend unit tests, all Go tests, and the five renderer tests pass. The full browser suite still contains two unrelated pre-existing assertions concerning the window title and settings shortcut behavior.

## Troubleshooting

Use these URLs while running the Vite frontend to compare modes:

```text
/?renderer=webgl&customGlyphs=true
/?renderer=webgl&customGlyphs=false
/?renderer=dom
/?renderer=auto&terminalMetrics
```

If a new pane is monochrome, inspect the child environment first. `TERM=dumb`, an empty `COLORTERM`, or an explicitly configured `NO_COLOR` explains missing ANSI colors. If a border is dotted across every horizontal segment, check `data-renderer`; that indicates the DOM comparison mode is active.

## Remaining considerations

- WebGL remains the visual default even at fractional device scale factors because it produced better box-drawing joins on the target setup.
- DOM remains useful for isolating xterm/WebGL issues and for environments where WebGL context creation fails.
- Font metrics and pane dimensions can still vary across monitors and WebView2 versions; use `terminalMetrics` when investigating a new DPI combination.
- Future xterm upgrades should rerun the renderer matrix before changing the default policy.
