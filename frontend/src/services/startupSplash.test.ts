import { describe, expect, it } from "vitest";
import type { Settings } from "./backend";
import { shouldShowStartupSplash, withStartupSplash } from "./startupSplash";

const settings: Settings = {
  defaultShell: {
    id: "powershell",
    name: "PowerShell",
    executable: "powershell.exe",
    arguments: [],
    available: true,
  },
  scrollback: 10000,
  terminalHistoryLines: 2000,
  fontSize: 13,
  theme: "dark",
  keybindings: {},
};

describe("startup splash setting", () => {
  it("shows by default when the setting is missing", () => {
    expect(shouldShowStartupSplash(settings)).toBe(true);
    expect(shouldShowStartupSplash({ hideStartupSplash: false })).toBe(true);
  });

  it("does not show before settings are loaded", () => {
    expect(shouldShowStartupSplash(undefined)).toBe(false);
  });

  it("does not show when hidden", () => {
    expect(shouldShowStartupSplash({ hideStartupSplash: true })).toBe(false);
  });

  it("toggles the setting without touching other settings", () => {
    const hidden = withStartupSplash(settings, false);
    expect(hidden).toEqual({ ...settings, hideStartupSplash: true });
    expect(settings.hideStartupSplash).toBeUndefined();
    expect(withStartupSplash(hidden, true).hideStartupSplash).toBe(false);
  });
});
