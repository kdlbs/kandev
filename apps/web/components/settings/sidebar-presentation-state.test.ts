import { describe, expect, it } from "vitest";
import { presentationPatch, rebasePresentation } from "./sidebar-presentation-state";

describe("sidebar presentation drafts", () => {
  const saved = { fast: false, style: "simple" as const };
  it("preserves dirty fields while following direct sidebar edits", () => {
    expect(
      rebasePresentation({ ...saved, style: "compact" }, saved, { fast: true, style: "simple" }),
    ).toEqual({ fast: true, style: "compact" });
  });
  it("submits only changed fields, including explicit false", () => {
    expect(presentationPatch(saved, { fast: true, style: "simple" })).toEqual({
      sidebar_fast_actions_enabled: false,
    });
    expect(presentationPatch(saved, saved)).toEqual({});
  });
  it("preserves edits made during a save", () => {
    expect(
      rebasePresentation(
        { fast: false, style: "compact" },
        { fast: true, style: "compact" },
        { fast: true, style: "compact" },
      ),
    ).toEqual({ fast: false, style: "compact" });
  });
});
