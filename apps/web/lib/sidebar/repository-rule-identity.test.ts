import { describe, expect, it } from "vitest";
import {
  repositoryIdentityForSavedRepository,
  repositoryIdentityForTaskRepository,
  repositoryRuleTargetMatches,
  type RepositoryRuleTarget,
} from "./repository-rule-identity";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const GITHUB = "github";
const GITHUB_HOST = "github.com";
const ACME = "acme";
const ACME_WIDGET = "acme/widget";

describe("repository rule identity", () => {
  it("prefers provider identity over workspace identity", () => {
    expect(
      repositoryIdentityForSavedRepository({
        id: "workspace-repo",
        workspace_id: WORKSPACE_A,
        provider: GITHUB,
        provider_repo_id: ACME_WIDGET,
        provider_host: GITHUB_HOST,
        provider_scope: ACME,
        local_path: "/tmp/widget",
      }),
    ).toEqual({
      kind: "provider",
      provider_id: GITHUB,
      host: GITHUB_HOST,
      scope: ACME,
      provider_repository_id: ACME_WIDGET,
    });
  });

  it("matches any attached repository and respects workspace scoping", () => {
    const target: RepositoryRuleTarget = {
      kind: "provider",
      provider_id: "github",
      host: "github.com",
      scope: "acme",
      provider_repository_id: "acme/widget",
    };
    expect(
      repositoryRuleTargetMatches(target, "workspace-b", [
        repositoryIdentityForTaskRepository({
          repository_id: "other",
          workspace_id: WORKSPACE_B,
          provider: GITHUB,
          provider_repo_id: ACME_WIDGET,
          provider_host: GITHUB_HOST,
          provider_scope: ACME,
          local_path: "/tmp/other",
        }),
      ]),
    ).toBe(true);

    expect(
      repositoryRuleTargetMatches(
        { kind: "workspace", workspace_id: "workspace-a", repository_id: "repo-1" },
        WORKSPACE_B,
        [{ kind: "workspace", workspace_id: WORKSPACE_B, repository_id: "repo-1" }],
      ),
    ).toBe(false);
  });

  it("normalizes local paths before comparing them", () => {
    const target: RepositoryRuleTarget = { kind: "local", path: "/work/project" };
    expect(
      repositoryRuleTargetMatches(target, "workspace-a", [
        { kind: "local", path: "/work/project/" },
      ]),
    ).toBe(true);
  });

  it("removes repeated trailing separators while preserving the root", () => {
    expect(
      repositoryIdentityForTaskRepository({
        repository_id: "repo-1",
        local_path: "/work/project///",
      }),
    ).toEqual({ kind: "local", path: "/work/project" });
    expect(
      repositoryIdentityForTaskRepository({
        repository_id: "repo-1",
        local_path: "\\work\\project\\\\",
      }),
    ).toEqual({ kind: "local", path: "/work/project" });
    expect(
      repositoryIdentityForTaskRepository({
        repository_id: "repo-1",
        local_path: "////",
      }),
    ).toEqual({ kind: "local", path: "/" });
  });
});
