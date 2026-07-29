import { describe, it, expect } from "vitest";
import {
  shouldAutoScrollOnMessagesChange,
  shouldAutoScrollOnWorkingStart,
  hasTranscriptProgressedPastView,
  shouldCatchUpOnAutoScrollEnable,
  resolveNativeInitialScrollTop,
  resolveFollowOutput,
  isPrependUpdate,
} from "./transcript-auto-scroll";

describe("isPrependUpdate", () => {
  it("is a prepend when item count grows and the first item's key changed", () => {
    expect(
      isPrependUpdate({
        prevItemCount: 20,
        nextItemCount: 40,
        prevFirstKey: "msg-20",
        nextFirstKey: "msg-0",
      }),
    ).toBe(true);
  });

  it("is NOT a prepend when item count grows but the first item's key is unchanged (append)", () => {
    expect(
      isPrependUpdate({
        prevItemCount: 20,
        nextItemCount: 21,
        prevFirstKey: "msg-0",
        nextFirstKey: "msg-0",
      }),
    ).toBe(false);
  });

  it("is not a prepend when item count did not grow", () => {
    expect(
      isPrependUpdate({
        prevItemCount: 20,
        nextItemCount: 20,
        prevFirstKey: "msg-0",
        nextFirstKey: "msg-5",
      }),
    ).toBe(false);
  });

  it("is not a prepend when there is no first item yet", () => {
    expect(
      isPrependUpdate({
        prevItemCount: 0,
        nextItemCount: 1,
        prevFirstKey: null,
        nextFirstKey: null,
      }),
    ).toBe(false);
  });
});

describe("shouldAutoScrollOnMessagesChange", () => {
  it("scrolls when enabled and near the bottom", () => {
    expect(shouldAutoScrollOnMessagesChange(true, true)).toBe(true);
  });

  it("does not scroll when enabled but scrolled away from the bottom", () => {
    expect(shouldAutoScrollOnMessagesChange(true, false)).toBe(false);
  });

  it("never scrolls while auto-scroll is disabled, even near the bottom", () => {
    expect(shouldAutoScrollOnMessagesChange(false, true)).toBe(false);
  });
});

describe("shouldAutoScrollOnWorkingStart", () => {
  it("forces scroll on turn start when enabled", () => {
    expect(shouldAutoScrollOnWorkingStart(true)).toBe(true);
  });

  it("suppresses the forced scroll when disabled", () => {
    expect(shouldAutoScrollOnWorkingStart(false)).toBe(false);
  });
});

describe("hasTranscriptProgressedPastView", () => {
  it("is false when the viewport bottom already matches the content bottom", () => {
    expect(
      hasTranscriptProgressedPastView({ scrollTop: 800, scrollHeight: 900, clientHeight: 100 }),
    ).toBe(false);
  });

  it("is false within the sub-pixel settle tolerance", () => {
    expect(
      hasTranscriptProgressedPastView({ scrollTop: 799, scrollHeight: 900, clientHeight: 100 }),
    ).toBe(false);
  });

  it("is true once content extends meaningfully past the current viewport", () => {
    expect(
      hasTranscriptProgressedPastView({ scrollTop: 500, scrollHeight: 900, clientHeight: 100 }),
    ).toBe(true);
  });
});

describe("shouldCatchUpOnAutoScrollEnable", () => {
  it("catches up when flipping from disabled to enabled with content below view", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({ wasEnabled: false, nowEnabled: true, isAtBottom: false }),
    ).toBe(true);
  });

  it("does not catch up when already at the bottom", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({ wasEnabled: false, nowEnabled: true, isAtBottom: true }),
    ).toBe(false);
  });

  it("does nothing when flipping from enabled to disabled", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({ wasEnabled: true, nowEnabled: false, isAtBottom: false }),
    ).toBe(false);
  });

  it("does nothing when the enabled state is unchanged", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({ wasEnabled: true, nowEnabled: true, isAtBottom: false }),
    ).toBe(false);
    expect(
      shouldCatchUpOnAutoScrollEnable({ wasEnabled: false, nowEnabled: false, isAtBottom: false }),
    ).toBe(false);
  });
});

describe("resolveNativeInitialScrollTop", () => {
  it("defers to a pending dockview layout restore regardless of the toggle", () => {
    expect(
      resolveNativeInitialScrollTop({
        enabled: false,
        hasPendingLayoutRestore: true,
        savedScrollTop: 42,
        scrollHeight: 900,
      }),
    ).toBeNull();
  });

  it("restores the saved offset when disabled", () => {
    expect(
      resolveNativeInitialScrollTop({
        enabled: false,
        hasPendingLayoutRestore: false,
        savedScrollTop: 250,
        scrollHeight: 900,
      }),
    ).toBe(250);
  });

  it("falls back to the bottom when disabled but nothing was ever captured", () => {
    expect(
      resolveNativeInitialScrollTop({
        enabled: false,
        hasPendingLayoutRestore: false,
        savedScrollTop: undefined,
        scrollHeight: 900,
      }),
    ).toBe(900);
  });

  it("scrolls to the bottom when enabled, ignoring any saved offset", () => {
    expect(
      resolveNativeInitialScrollTop({
        enabled: true,
        hasPendingLayoutRestore: false,
        savedScrollTop: 250,
        scrollHeight: 900,
      }),
    ).toBe(900);
  });
});

describe("resolveFollowOutput", () => {
  it("never follows while auto-scroll is disabled", () => {
    expect(resolveFollowOutput(false, true)).toBe(false);
    expect(resolveFollowOutput(false, false)).toBe(false);
  });

  it("follows smoothly when enabled and at the bottom", () => {
    expect(resolveFollowOutput(true, true)).toBe("smooth");
  });

  it("does not follow when enabled but scrolled away from the bottom", () => {
    expect(resolveFollowOutput(true, false)).toBe(false);
  });
});
