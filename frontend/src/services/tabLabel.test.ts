import { expect, test } from "vitest";
import type { Tab } from "./backend";
import { tabLabel } from "./tabLabel";

test("automatic tabs use a two-digit tab name", () => {
  expect(
    tabLabel({ title: "PowerShell", titleMode: "automatic" } as Tab, 0),
  ).toBe("Tab 01");
  expect(
    tabLabel({ title: "Terminal", titleMode: "automatic" } as Tab, 11),
  ).toBe("Tab 12");
});

test("manually renamed tabs keep their custom name", () => {
  expect(
    tabLabel({ title: "Build & test", titleMode: "manual" } as Tab, 0),
  ).toBe("Build & test");
});
