import { describe, it, expect } from "vitest";
import { isPassthroughSession } from "./is-passthrough-session";

describe("isPassthroughSession", () => {
  it("returns true when is_passthrough is true even without a snapshot", () => {
    expect(isPassthroughSession({ is_passthrough: true })).toBe(true);
  });

  it("returns false when is_passthrough is false even if snapshot says passthrough", () => {
    expect(
      isPassthroughSession({
        is_passthrough: false,
        agent_profile_snapshot: { cli_passthrough: true },
      }),
    ).toBe(false);
  });

  it("falls back to snapshot.cli_passthrough when is_passthrough is undefined", () => {
    expect(
      isPassthroughSession({
        agent_profile_snapshot: { cli_passthrough: true },
      }),
    ).toBe(true);
    expect(
      isPassthroughSession({
        agent_profile_snapshot: { cli_passthrough: false },
      }),
    ).toBe(false);
  });

  it("returns false when session is missing or has no passthrough signals", () => {
    expect(isPassthroughSession(null)).toBe(false);
    expect(isPassthroughSession(undefined)).toBe(false);
    expect(isPassthroughSession({})).toBe(false);
    expect(isPassthroughSession({ agent_profile_snapshot: null })).toBe(false);
  });
});
