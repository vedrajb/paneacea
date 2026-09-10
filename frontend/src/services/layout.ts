import type { Layout } from "./backend";
export type Rect = {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
};
export function rectangles(
  node: Layout,
  x = 0,
  y = 0,
  width = 1,
  height = 1,
): Rect[] {
  if (node.paneId) return [{ id: node.paneId, x, y, width, height }];
  if (!node.first || !node.second) return [];
  const ratio = node.ratio ?? 0.5;
  return node.orientation === "vertical"
    ? [
        ...rectangles(node.first, x, y, width * ratio, height),
        ...rectangles(
          node.second,
          x + width * ratio,
          y,
          width * (1 - ratio),
          height,
        ),
      ]
    : [
        ...rectangles(node.first, x, y, width, height * ratio),
        ...rectangles(
          node.second,
          x,
          y + height * ratio,
          width,
          height * (1 - ratio),
        ),
      ];
}
export function neighbor(
  node: Layout,
  id: string,
  direction: string,
): string | null {
  const rects = rectangles(node),
    current = rects.find((r) => r.id === id);
  if (!current) return null;
  const horizontal = direction === "Left" || direction === "Right",
    sign = direction === "Left" || direction === "Up" ? -1 : 1;
  const center = (r: Rect) =>
    horizontal ? r.x + r.width / 2 : r.y + r.height / 2;
  const cross = (r: Rect) =>
    horizontal ? r.y + r.height / 2 : r.x + r.width / 2;
  return (
    rects
      .filter(
        (r) => r.id !== id && (center(r) - center(current)) * sign > 0.0001,
      )
      .sort(
        (a, b) =>
          Math.abs(center(a) - center(current)) +
          Math.abs(cross(a) - cross(current)) * 2 -
          (Math.abs(center(b) - center(current)) +
            Math.abs(cross(b) - cross(current)) * 2),
      )[0]?.id ?? null
  );
}
export function resizeTarget(
  node: Layout,
  id: string,
  direction: string,
  path: number[] = [],
): { path: number[]; ratio: number } | null {
  if (node.paneId) return null;
  const first = rectangles(node.first!).some((r) => r.id === id),
    child = first ? node.first : node.second;
  if (!child) return null;
  const nested = resizeTarget(child, id, direction, [...path, first ? 0 : 1]);
  if (nested) return nested;
  const orientation =
    direction === "Left" || direction === "Right" ? "vertical" : "horizontal";
  if (node.orientation !== orientation) return null;
  return {
    path,
    ratio: Math.min(
      0.9,
      Math.max(
        0.1,
        (node.ratio ?? 0.5) +
          (direction === "Left" || direction === "Up" ? -0.05 : 0.05),
      ),
    ),
  };
}
