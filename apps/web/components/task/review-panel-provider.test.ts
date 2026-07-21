import { describe, expect, it } from "vitest";
import { resolveReviewPanelProvider } from "./review-panel-provider";

describe("resolveReviewPanelProvider", () => {
  it("routes a GitLab-stamped panel to merge request detail", () => {
    expect(
      resolveReviewPanelProvider({ provider: "gitlab", mrKey: "gitlab.com|a/b|7" }, true, true),
    ).toBe("gitlab");
  });

  it("preserves the legacy GitHub panel when both providers are linked", () => {
    expect(resolveReviewPanelProvider({}, true, true)).toBe("github");
  });

  it("falls back to GitLab when the task has no GitHub pull request", () => {
    expect(resolveReviewPanelProvider({}, false, true)).toBe("gitlab");
  });
});
