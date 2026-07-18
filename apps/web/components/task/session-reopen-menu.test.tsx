import { describe, expect, it } from "vitest";
import { shouldShowReopenStateIcon } from "./session-reopen-menu";

describe("shouldShowReopenStateIcon", () => {
  it("surfaces the icon for a background-running session (RUNNING + background)", () => {
    // The defect this fixes: a session whose foreground turn is idle while
    // background work runs previously showed no icon (state dropped). It must
    // now render — the shared background-running spinner, never a done check.
    expect(shouldShowReopenStateIcon("RUNNING", "background")).toBe(true);
  });

  it("keeps a generating RUNNING session icon-less (established silent affordance)", () => {
    expect(shouldShowReopenStateIcon("RUNNING", "generating")).toBe(false);
  });

  it("falls back to silence — not done — when a RUNNING substate is unknown", () => {
    // §req safe-defaults: an unknown substate on a live session must never
    // resolve to a done affordance. Silence (no icon) is the safe reading here.
    expect(shouldShowReopenStateIcon("RUNNING", null)).toBe(false);
    expect(shouldShowReopenStateIcon("RUNNING", undefined)).toBe(false);
  });

  it("keeps STARTING and WAITING_FOR_INPUT icon-less", () => {
    expect(shouldShowReopenStateIcon("STARTING", null)).toBe(false);
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null)).toBe(false);
  });

  it("renders the existing icon for terminal / other states", () => {
    expect(shouldShowReopenStateIcon("COMPLETED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("FAILED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("CANCELLED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("CREATED", null)).toBe(true);
  });
});
