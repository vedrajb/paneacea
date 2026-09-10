import { it, expect } from "vitest";
import { neighbor, resizeTarget, rectangles } from "./layout";
import type { Layout } from "./backend";
const tree: Layout = {
  orientation: "vertical",
  ratio: 0.6,
  first: { paneId: "a" },
  second: {
    orientation: "horizontal",
    ratio: 0.5,
    first: { paneId: "b" },
    second: { paneId: "c" },
  },
};
it("navigates nested splits by direction", () => {
  expect(neighbor(tree, "b", "Down")).toBe("c");
  expect(neighbor(tree, "c", "Left")).toBe("a");
  expect(neighbor(tree, "a", "Left")).toBeNull();
});
it("resizes nearest ancestor with matching orientation", () => {
  expect(resizeTarget(tree, "c", "Up")).toEqual({ path: [1], ratio: 0.45 });
  expect(resizeTarget(tree, "c", "Right")?.path).toEqual([]);
});
it("partitions the complete terminal area", () => {
  expect(
    rectangles(tree).reduce((sum, r) => sum + r.width * r.height, 0),
  ).toBeCloseTo(1);
});
