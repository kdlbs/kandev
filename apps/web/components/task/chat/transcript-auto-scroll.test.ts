import { describe, it, expect } from "vitest";
import {
  shouldAutoScrollOnMessagesChange,
  shouldAutoScrollOnWorkingStart,
  hasTranscriptProgressedPastView,
  hasTranscriptAppendedSinceBaseline,
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

const BASELINE_UPDATED_AT = "2026-01-01T00:00:00Z";

const baseline = {
  baselineCount: 20,
  baselineLastId: "msg-19",
  baselineLastUpdatedAt: BASELINE_UPDATED_AT,
};

describe("hasTranscriptAppendedSinceBaseline", () => {
  it("is true when a new row was appended (count grew and the last id changed)", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        ...baseline,
        currentCount: 21,
        currentLastId: "msg-20",
        currentLastUpdatedAt: "2026-01-01T00:00:05Z",
      }),
    ).toBe(true);
  });

  it("is true when the same last row streamed new content (id unchanged, updated_at advanced)", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        ...baseline,
        currentCount: 20,
        currentLastId: "msg-19",
        currentLastUpdatedAt: "2026-01-01T00:00:05Z",
      }),
    ).toBe(true);
  });

  it("is false for a prepend-only load (count grew but the last id and its updated_at are unchanged)", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        ...baseline,
        currentCount: 40,
        currentLastId: "msg-19",
        currentLastUpdatedAt: BASELINE_UPDATED_AT,
      }),
    ).toBe(false);
  });

  it("is false when nothing changed", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        ...baseline,
        currentCount: 20,
        currentLastId: "msg-19",
        currentLastUpdatedAt: BASELINE_UPDATED_AT,
      }),
    ).toBe(false);
  });

  it("does not treat a missing current updated_at as a change from a defined baseline", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        ...baseline,
        currentCount: 20,
        currentLastId: "msg-19",
        currentLastUpdatedAt: undefined,
      }),
    ).toBe(false);
  });

  it("is false when there is no current last id yet", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        baselineCount: 0,
        baselineLastId: null,
        baselineLastUpdatedAt: undefined,
        currentCount: 0,
        currentLastId: null,
        currentLastUpdatedAt: undefined,
      }),
    ).toBe(false);
  });

  it("is true for the very first message appearing", () => {
    expect(
      hasTranscriptAppendedSinceBaseline({
        baselineCount: 0,
        baselineLastId: null,
        baselineLastUpdatedAt: undefined,
        currentCount: 1,
        currentLastId: "msg-0",
        currentLastUpdatedAt: BASELINE_UPDATED_AT,
      }),
    ).toBe(true);
  });
});

describe("shouldCatchUpOnAutoScrollEnable", () => {
  it("catches up when content was appended while disabled and it is not yet in view", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: false,
        nowEnabled: true,
        appendedSinceDisable: true,
        isAtBottom: false,
      }),
    ).toBe(true);
  });

  it("does NOT catch up when the user was already scrolled away from the bottom before disabling and nothing new arrived (regression: disable while already scrolled up)", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: false,
        nowEnabled: true,
        appendedSinceDisable: false,
        isAtBottom: false,
      }),
    ).toBe(false);
  });

  it("does NOT catch up when the user scrolled further away themselves while disabled, with no new content (regression: manual scroll is not progression)", () => {
    // appendedSinceDisable is derived purely from message identity/updated_at
    // — a manual scroll can never flip it, unlike a scrollTop/height-based
    // baseline would.
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: false,
        nowEnabled: true,
        appendedSinceDisable: false,
        isAtBottom: false,
      }),
    ).toBe(false);
  });

  it("does not catch up when content appended but the user already scrolled down to see it", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: false,
        nowEnabled: true,
        appendedSinceDisable: true,
        isAtBottom: true,
      }),
    ).toBe(false);
  });

  it("does nothing when flipping from enabled to disabled", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: true,
        nowEnabled: false,
        appendedSinceDisable: true,
        isAtBottom: false,
      }),
    ).toBe(false);
  });

  it("does nothing when the enabled state is unchanged", () => {
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: true,
        nowEnabled: true,
        appendedSinceDisable: true,
        isAtBottom: false,
      }),
    ).toBe(false);
    expect(
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled: false,
        nowEnabled: false,
        appendedSinceDisable: true,
        isAtBottom: false,
      }),
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
