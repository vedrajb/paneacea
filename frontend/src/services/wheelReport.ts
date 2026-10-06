export type WheelInput = Pick<
  WheelEvent,
  | "deltaY"
  | "deltaMode"
  | "clientX"
  | "clientY"
  | "shiftKey"
  | "altKey"
  | "ctrlKey"
  | "metaKey"
>;
export type ScreenBox = { left: number; top: number; width: number; height: number };

const DOM_DELTA_PIXEL = 0;
const DOM_DELTA_PAGE = 2;

// Builds one SGR wheel report per event, accumulating pixel deltas the same way xterm.js does.
export function mouseWheelReport(
  event: WheelInput,
  screen: ScreenBox,
  columns: number,
  rows: number,
  partial: number,
): { report: string; partial: number } {
  if (event.deltaY === 0 || columns <= 0 || rows <= 0 || screen.height <= 0)
    return { report: "", partial };
  let lines = event.deltaY;
  if (event.deltaMode === DOM_DELTA_PIXEL) {
    let amount = event.deltaY / (screen.height / rows);
    if (Math.abs(event.deltaY) < 50) amount *= 0.3;
    partial += amount;
    lines = Math.trunc(partial);
    partial %= 1;
  } else if (event.deltaMode === DOM_DELTA_PAGE) {
    lines = event.deltaY * rows;
  }
  if (lines === 0) return { report: "", partial };
  const button =
    (lines < 0 ? 64 : 65) +
    (event.shiftKey ? 4 : 0) +
    (event.altKey || event.metaKey ? 8 : 0) +
    (event.ctrlKey ? 16 : 0);
  const cell = (offset: number, size: number, count: number) =>
    Math.min(count, Math.max(1, Math.floor((offset / size) * count) + 1));
  const column = cell(event.clientX - screen.left, screen.width, columns);
  const row = cell(event.clientY - screen.top, screen.height, rows);
  return { report: `\x1b[<${button};${column};${row}M`, partial };
}
