import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { pickRepoLabel, resolvePullRequestBaseBranch } from "./vcs-dialogs";

/**
 * `pickRepoLabel` feeds the commit and change-request dialog titles, which
 * render it through `integrations:commitChangesScoped` / `createScoped`. Two of
 * its three branches used to return hardcoded English ("All repos",
 * "Repository") inside those now-translated titles; these cases pin that they
 * resolve through the catalog while the repo-name branches keep returning
 * user data verbatim.
 *
 * `vitest.setup.ts` initializes i18next in `en`, so `t()` returns the English
 * catalog values here.
 */
describe("pickRepoLabel", () => {
  const noDisplayName = () => undefined;

  it("prefers the resolved display name for an explicitly scoped repo", () => {
    expect(pickRepoLabel("api", false, (name) => `kdlbs/${name}`, t)).toBe("kdlbs/api");
  });

  it("falls back to the raw repo name when it has no display name", () => {
    // Repository names are user data and must never be translated.
    expect(pickRepoLabel("api", false, noDisplayName, t)).toBe("api");
  });

  it("translates the multi-repo fan-out label when no repo is scoped", () => {
    expect(pickRepoLabel(undefined, true, noDisplayName, t)).toBe(t("integrations:allRepos"));
  });

  it("translates an explicit workspace-root scope separately from fan-out", () => {
    expect(pickRepoLabel("", true, noDisplayName, t)).toBe(t("integrations:repository"));
  });

  it("translates the single-repo fallback when the primary repo has no display name", () => {
    expect(pickRepoLabel(undefined, false, noDisplayName, t)).toBe(t("integrations:repository"));
  });

  it("prefers the primary repo's display name over the translated fallback", () => {
    expect(pickRepoLabel("", false, () => "kdlbs/kandev", t)).toBe("kdlbs/kandev");
  });
});

describe("resolvePullRequestBaseBranch", () => {
  it("uses the selected repository policy target in a multi-repo task", () => {
    expect(
      resolvePullRequestBaseBranch("frontend", { backend: "develop", frontend: "main" }, "develop"),
    ).toBe("main");
  });

  it("uses the selected repository policy target for a branch worktree label", () => {
    expect(
      resolvePullRequestBaseBranch("frontend · branch-2", { frontend: "main" }, "develop"),
    ).toBe("main");
  });

  it("falls back to the task target when the selected repository has no policy", () => {
    expect(resolvePullRequestBaseBranch("frontend", { backend: "main" }, "develop")).toBe(
      "develop",
    );
  });

  it("does not use a policy from a repository with a shared name prefix", () => {
    expect(resolvePullRequestBaseBranch("api-gateway-x", { api: "develop" }, "main")).toBe("main");
  });
});
