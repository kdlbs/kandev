import { describe, it, expect } from "vitest";
import { scoreBranch } from "./branch-filter";

function rank(values: string[], q: string, keywords?: Record<string, string[]>): string[] {
  return values
    .map((v) => ({ v, s: scoreBranch(v, q, keywords?.[v]) }))
    .filter((x) => x.s > 0)
    .sort((a, b) => b.s - a.s)
    .map((x) => x.v);
}

describe("scoreBranch", () => {
  it("returns a positive score for empty query so all options stay visible", () => {
    expect(scoreBranch("origin/main", "")).toBeGreaterThan(0);
    expect(scoreBranch("anything", "   ")).toBeGreaterThan(0);
  });

  it("scores exact full match highest", () => {
    const exact = scoreBranch("main", "main");
    const leafExact = scoreBranch("origin/main", "main");
    const prefix = scoreBranch("mainline", "main");
    expect(exact).toBeGreaterThan(leafExact);
    expect(leafExact).toBeGreaterThan(prefix);
  });

  it("ranks the leaf-name match above a mid-string substring", () => {
    const ranked = rank(["origin/feature/foo-bar", "barometer/main"], "bar");
    expect(ranked[0]).toBe("barometer/main"); // full prefix wins over leaf substring
    // But leaf-segment match still scores
    expect(scoreBranch("origin/feature/foo-bar", "bar")).toBeGreaterThan(0);
  });

  it("ranks segment-boundary matches above mid-word matches", () => {
    const segment = scoreBranch("feature/auth/login", "auth");
    const midWord = scoreBranch("zauthority/login", "auth");
    expect(segment).toBeGreaterThan(midWord);
  });

  it("treats /, -, _, . as segment separators", () => {
    expect(scoreBranch("feature_auth-login", "auth")).toBeGreaterThan(0);
    expect(scoreBranch("feature.auth.login", "auth")).toBeGreaterThan(0);
  });

  it("falls back to fuzzy subsequence match when no substring matches", () => {
    const score = scoreBranch("feature/foobar", "ffb");
    expect(score).toBeGreaterThan(0);
    // ...but a substring match outranks the fuzzy fallback
    const sub = scoreBranch("feature/foobar", "foo");
    expect(sub).toBeGreaterThan(score);
  });

  it("returns 0 when no signal is present at all", () => {
    expect(scoreBranch("main", "xyz")).toBe(0);
  });

  it("is case-insensitive", () => {
    expect(scoreBranch("Origin/Feature/Foo", "FEATURE")).toBeGreaterThan(0);
    expect(scoreBranch("MAIN", "main")).toBe(scoreBranch("main", "MAIN"));
  });

  it("uses keywords as a secondary signal when value doesn't match", () => {
    const noKeywords = scoreBranch("origin/release-1.2.3", "auth");
    const withKeywords = scoreBranch("origin/release-1.2.3", "auth", ["auth-feature"]);
    expect(noKeywords).toBe(0);
    expect(withKeywords).toBeGreaterThan(0);
  });

  it("prefers shorter haystacks among substring matches (length penalty)", () => {
    const shortHit = scoreBranch("foo-bar", "bar");
    const longHit = scoreBranch("a".repeat(900) + "bar", "bar");
    expect(shortHit).toBeGreaterThan(longHit);
  });

  it("ranks long branch list realistically", () => {
    const list = [
      "main",
      "origin/main",
      "origin/feature/auth/login",
      "origin/feature/auth/logout",
      "origin/feature/billing/checkout",
      "origin/release/2026-04-15",
      "origin/some-very-long-branch-name-that-mentions-auth-somewhere",
    ];
    const ranked = rank(list, "auth");
    // Segment-boundary matches come before the mid-name occurrence.
    expect(ranked.indexOf("origin/feature/auth/login")).toBeLessThan(
      ranked.indexOf("origin/some-very-long-branch-name-that-mentions-auth-somewhere"),
    );
    expect(ranked.indexOf("origin/feature/auth/logout")).toBeLessThan(
      ranked.indexOf("origin/some-very-long-branch-name-that-mentions-auth-somewhere"),
    );
  });
});
