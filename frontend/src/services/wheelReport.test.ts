import { describe, expect, it } from "vitest";
import { mouseWheelReport, type WheelInput } from "./wheelReport";

const screen = { left: 10, top: 20, width: 800, height: 400 };
const wheel = (overrides: Partial<WheelInput>): WheelInput => ({
  deltaY: 0,
  deltaMode: 0,
  clientX: 10,
  clientY: 20,
  shiftKey: false,
  altKey: false,
  ctrlKey: false,
  metaKey: false,
  ...overrides,
});

describe("mouseWheelReport", () => {
  it("reports a mouse notch up as an SGR wheel-up at the pointer cell", () => {
    const result = mouseWheelReport(
      wheel({ deltaY: -100, clientX: 10 + 85, clientY: 20 + 45 }),
      screen,
      80,
      20,
      0,
    );
    expect(result.report).toBe("\x1b[<64;9;3M");
  });

  it("reports wheel-down with button 65", () => {
    expect(
      mouseWheelReport(wheel({ deltaY: 100 }), screen, 80, 20, 0).report,
    ).toBe("\x1b[<65;1;1M");
  });

  it("clamps coordinates to the terminal grid", () => {
    expect(
      mouseWheelReport(
        wheel({ deltaY: 3, deltaMode: 1, clientX: 5000, clientY: 5000 }),
        screen,
        80,
        20,
        0,
      ).report,
    ).toBe("\x1b[<65;80;20M");
  });

  it("adds SGR modifier bits for shift and alt", () => {
    expect(
      mouseWheelReport(
        wheel({ deltaY: -3, deltaMode: 1, shiftKey: true, altKey: true }),
        screen,
        80,
        20,
        0,
      ).report,
    ).toBe("\x1b[<76;1;1M");
  });

  it("accumulates small trackpad deltas before reporting", () => {
    let partial = 0;
    let reports = 0;
    for (let index = 0; index < 40; index++) {
      const result = mouseWheelReport(
        wheel({ deltaY: 4 }),
        screen,
        80,
        20,
        partial,
      );
      partial = result.partial;
      if (result.report) reports++;
    }
    expect(reports).toBe(2);
  });

  it("ignores horizontal-only wheel events", () => {
    expect(mouseWheelReport(wheel({ deltaY: 0 }), screen, 80, 20, 0)).toEqual({
      report: "",
      partial: 0,
    });
  });
});
