import type { Settings } from "./backend";

export function shouldShowStartupSplash(
  settings: Pick<Settings, "hideStartupSplash"> | undefined,
): boolean {
  return !!settings && !settings.hideStartupSplash;
}

export function withStartupSplash(
  settings: Settings,
  show: boolean,
): Settings {
  return { ...settings, hideStartupSplash: !show };
}
