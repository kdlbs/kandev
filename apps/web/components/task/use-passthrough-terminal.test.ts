import { describe, expect, it } from "vitest";
import { reconnectDelayMs } from "./ws-reconnect";

describe("reconnectDelayMs", () => {
  it("returns 300ms for attempt 0", () => {
    expect(reconnectDelayMs(0)).toBe(300);
  });

  it("doubles delay for each attempt", () => {
    expect(reconnectDelayMs(0)).toBe(300);
    expect(reconnectDelayMs(1)).toBe(600);
    expect(reconnectDelayMs(2)).toBe(1200);
    expect(reconnectDelayMs(3)).toBe(2400);
    expect(reconnectDelayMs(4)).toBe(4800);
  });

  it("caps at 5000ms", () => {
    expect(reconnectDelayMs(5)).toBe(5000);
  });

  it("caps attempt at 5 so high values stay at 5000ms", () => {
    expect(reconnectDelayMs(10)).toBe(5000);
    expect(reconnectDelayMs(100)).toBe(5000);
  });
});
