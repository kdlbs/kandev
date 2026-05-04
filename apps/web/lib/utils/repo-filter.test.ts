import { describe, it, expect } from "vitest";
import { scoreRepo } from "./repo-filter";

describe("scoreRepo", () => {
  it("returns a positive score for an exact keyword match", () => {
    expect(scoreRepo("uuid-123", "arthur", ["arthur"])).toBeGreaterThan(0);
  });

  it("returns a positive score for a substring/segment hit on the value", () => {
    expect(scoreRepo("acme/widget-config", "widget")).toBeGreaterThan(0);
  });

  it("floors weak subsequence-only matches that scoreBranch would surface", () => {
    // a-r-t-h-u-r appears in order across the path but no segment contains
    // "arthur" — exactly the false positive scoreRepo is designed to kill.
    expect(scoreRepo("playground/fun/thm/gameserver/lxd-alpine-builder", "arthur")).toBe(0);
  });

  it("returns a positive score when the search prefixes the value's leaf", () => {
    expect(scoreRepo("playground/sky/arthur", "arth")).toBeGreaterThan(0);
  });

  it("returns 0 when nothing in value or keywords resembles the search", () => {
    expect(scoreRepo("playground/foo", "completely-unrelated", ["foo"])).toBe(0);
  });
});
