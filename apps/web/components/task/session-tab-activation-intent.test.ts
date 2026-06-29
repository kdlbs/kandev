import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearSessionTabUserActivationIntentsForTest,
  consumeSessionTabUserActivationIntent,
  markSessionTabUserActivationIntent,
  shouldMarkSessionTabUserActivationIntent,
} from "./session-tab-activation-intent";

describe("session tab activation intent", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
    clearSessionTabUserActivationIntentsForTest();
  });

  afterEach(() => {
    clearSessionTabUserActivationIntentsForTest();
    vi.useRealTimers();
  });

  it("ignores null and undefined session ids", () => {
    markSessionTabUserActivationIntent(null);
    markSessionTabUserActivationIntent(undefined);

    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(false);
  });

  it("expires marked intent after the TTL", () => {
    markSessionTabUserActivationIntent("s-active");

    vi.advanceTimersByTime(1501);

    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(false);
  });

  it("preserves one session intent when another session is consumed", () => {
    markSessionTabUserActivationIntent("s-active");

    expect(consumeSessionTabUserActivationIntent("s-other")).toBe(false);
    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(true);
  });

  it("consumes a matching intent once", () => {
    markSessionTabUserActivationIntent("s-active");

    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(true);
    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(false);
  });

  it("keeps independent intents for fast successive tab activations", () => {
    markSessionTabUserActivationIntent("s-first");
    markSessionTabUserActivationIntent("s-second");

    expect(consumeSessionTabUserActivationIntent("s-first")).toBe(true);
    expect(consumeSessionTabUserActivationIntent("s-second")).toBe(true);
  });

  it("does not mark intent for already-active tabs", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: "s-active",
        isActive: true,
        target: document.createElement("span"),
      }),
    ).toBe(false);
  });

  it("does not mark intent from nested interactive controls", () => {
    const button = document.createElement("button");
    const label = document.createElement("span");
    button.append(label);

    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: "s-inactive",
        isActive: false,
        target: label,
      }),
    ).toBe(false);
  });

  it("marks intent for inactive tab activation surfaces", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: "s-inactive",
        isActive: false,
        target: document.createElement("span"),
      }),
    ).toBe(true);
  });
});
