import { describe, expect, it } from "vitest";
import { orgAllowedByScope, repoAllowedByScope } from "./repo-scope";
import type { GitHubWorkspaceSettings } from "@/lib/types/github";

function settings(
  mode: string,
  orgs: string[] = [],
  repos: Array<{ owner: string; name: string }> = [],
): GitHubWorkspaceSettings {
  return {
    workspace_id: "ws-1",
    repo_scope_mode: mode,
    repo_scope_orgs: orgs,
    repo_scope_repos: repos,
  } as unknown as GitHubWorkspaceSettings;
}

describe("GitHub repository scope", () => {
  it("allows the union of selected owners and exact repositories", () => {
    const scope = settings("selected", [" Acme "], [{ owner: "Outside", name: "Tool" }]);

    expect(repoAllowedByScope(scope, "acme", "service")).toBe(true);
    expect(repoAllowedByScope(scope, "outside", "tool")).toBe(true);
    expect(repoAllowedByScope(scope, "outside", "private")).toBe(false);
  });

  it("denies every repository when selected scope is empty", () => {
    expect(repoAllowedByScope(settings("selected"), "acme", "service")).toBe(false);
  });

  it("allows an owner wildcard only when that owner is selected", () => {
    const scope = settings("selected", ["acme"], [{ owner: "outside", name: "tool" }]);

    expect(orgAllowedByScope(scope, " ACME ")).toBe(true);
    expect(orgAllowedByScope(scope, "outside")).toBe(false);
    expect(
      orgAllowedByScope(settings("selected", [], [{ owner: "outside", name: "tool" }]), "outside"),
    ).toBe(false);
  });

  it("keeps all-scope and missing-settings behavior", () => {
    expect(repoAllowedByScope(settings("all"), "acme", "service")).toBe(true);
    expect(orgAllowedByScope(settings("all"), "acme")).toBe(true);
    expect(repoAllowedByScope(null, "acme", "service")).toBe(true);
    expect(orgAllowedByScope(undefined, "acme")).toBe(true);
  });

  it("denies unsupported scope modes even when legacy selections are populated", () => {
    const scope = settings("future-policy", ["acme"], [{ owner: "outside", name: "tool" }]);

    expect(repoAllowedByScope(scope, "acme", "service")).toBe(false);
    expect(repoAllowedByScope(scope, "outside", "tool")).toBe(false);
    expect(orgAllowedByScope(scope, "acme")).toBe(false);
  });
});
