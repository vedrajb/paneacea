import { expect, test } from "vitest";
import { countPanesForTab } from "./tabPaneCount";

test("counts only panes belonging to the requested tab", () => {
  expect(
    countPanesForTab(
      {
        first: { tabId: "tab-1" },
        second: { tabId: "tab-1" },
        third: { tabId: "tab-2" },
      },
      "tab-1",
    ),
  ).toBe(2);
});
