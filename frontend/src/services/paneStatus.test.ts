import { expect, test } from "vitest";
import { visiblePaneState } from "./paneStatus";

test("hides normal and exited terminal states", () => {
  expect(visiblePaneState("running")).toBe("");
  expect(visiblePaneState("exited")).toBe("");
});

test("shows terminal errors and agent states", () => {
  expect(visiblePaneState("error")).toBe("error");
  expect(visiblePaneState("running", "working")).toBe("working");
  expect(visiblePaneState("running", "waiting")).toBe("waiting");
});
