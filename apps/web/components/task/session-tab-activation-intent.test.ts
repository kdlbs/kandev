import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearSessionTabUserActivationIntentsForTest,
  consumeSessionTabUserActivationIntent,
  markSessionPanelUserActivationIntent,
  markSessionTabUserActivationIntent,
  shouldMarkSessionTabUserActivationIntent,
} from "./session-tab-activation-intent";

const INACTIVE_SESSION_ID = "s-inactive";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
  clearSessionTabUserActivationIntentsForTest();
});

afterEach(() => {
  clearSessionTabUserActivationIntentsForTest();
  vi.useRealTimers();
});

describe("session tab activation intent lifecycle", () => {
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

  it("marks session intent from a session panel id", () => {
    markSessionPanelUserActivationIntent("session:s-active");
    markSessionPanelUserActivationIntent("files");

    expect(consumeSessionTabUserActivationIntent("s-active")).toBe(true);
    expect(consumeSessionTabUserActivationIntent("files")).toBe(false);
  });
});

describe("session tab activation intent guard", () => {
  it("does not mark intent for already-active tabs", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: "s-active",
        activeSessionId: "s-other",
        isActive: true,
        target: document.createElement("span"),
      }),
    ).toBe(false);
  });

  it("does not mark intent for the current active session when another panel is active", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: "s-active",
        activeSessionId: "s-active",
        isActive: false,
        target: document.createElement("span"),
      }),
    ).toBe(false);
  });

  it("does not mark intent without a session id", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: null,
        activeSessionId: "s-active",
        isActive: false,
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
        sessionId: INACTIVE_SESSION_ID,
        activeSessionId: "s-active",
        isActive: false,
        target: label,
      }),
    ).toBe(false);
  });

  it("does not mark intent from role-based nested interactive controls", () => {
    const button = document.createElement("span");
    button.setAttribute("role", "button");
    const label = document.createElement("span");
    button.append(label);

    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: INACTIVE_SESSION_ID,
        activeSessionId: "s-active",
        isActive: false,
        target: label,
      }),
    ).toBe(false);
  });

  it("does not mark intent from Dockview close actions", () => {
    const closeAction = document.createElement("div");
    closeAction.className = "dv-default-tab-action";
    const label = document.createElement("span");
    closeAction.append(label);

    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: INACTIVE_SESSION_ID,
        activeSessionId: "s-active",
        isActive: false,
        target: label,
      }),
    ).toBe(false);
  });

  it("marks intent for non-element targets on inactive tabs", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: INACTIVE_SESSION_ID,
        activeSessionId: "s-active",
        isActive: false,
        target: null,
      }),
    ).toBe(true);
  });

  it("marks intent for inactive tab activation surfaces", () => {
    expect(
      shouldMarkSessionTabUserActivationIntent({
        sessionId: INACTIVE_SESSION_ID,
        activeSessionId: "s-active",
        isActive: false,
        target: document.createElement("span"),
      }),
    ).toBe(true);
  });
});
