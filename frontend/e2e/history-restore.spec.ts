import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

test("saved Git Bash output survives a fresh ConPTY stream in xterm", async ({
  page,
}, testInfo) => {
  test.setTimeout(60000);
  const fixturePath = testInfo.outputPath("history-replay.json");
  mkdirSync(dirname(fixturePath), { recursive: true });
  execFileSync(
    "go",
    [
      "test",
      "./internal/daemon",
      "-run",
      "^TestBashHistorySurvivesStartupOutput$",
      "-count=1",
    ],
    {
      cwd: resolve(process.cwd(), ".."),
      env: { ...process.env, PANEACEA_HISTORY_FIXTURE: fixturePath },
      timeout: 45000,
    },
  );
  const fixture = JSON.parse(readFileSync(fixturePath, "utf8"));
  await page.goto("/");
  const rendered = await page.evaluate(async (fixture) => {
    const modulePath = "/node_modules/@xterm/xterm/lib/xterm.mjs";
    const { Terminal } = await import(modulePath);
    const terminal = new Terminal({
      cols: fixture.attached.columns,
      rows: fixture.attached.rows,
      scrollback: 10000,
    });
    const host = document.createElement("div");
    document.body.appendChild(host);
    terminal.open(host);
    const write = (encoded: string) =>
      new Promise<void>((resolve) =>
        terminal.write(
          Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0)),
          resolve,
        ),
      );
    const contents = () =>
      Array.from({ length: terminal.buffer.active.length }, (_, i) =>
        terminal.buffer.active.getLine(i).translateToString(true),
      ).join("\n");
    await write(fixture.attached.data);
    const before = contents();
    terminal.resize(81, 45);
    await write(fixture.startup);
    const after = contents();
    terminal.dispose();
    host.remove();
    return { before, after };
  }, fixture);
  expect(rendered.before).toContain("SAVED_BEFORE_RESTART");
  expect(rendered.after).toContain("FRESH_AFTER_RESTART");
  expect(rendered.after).toContain("SAVED_BEFORE_RESTART");
});
