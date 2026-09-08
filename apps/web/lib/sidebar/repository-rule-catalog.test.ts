import { describe, expect, it } from "vitest";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { Repository } from "@/lib/types/http";
import { buildRepositoryRuleCatalog, repositoryRuleTargetKey } from "./repository-rule-catalog";

const savedRepository = (overrides: Partial<Repository>): Repository =>
  ({
    id: "repo-1",
    workspace_id: "workspace-1",
    name: "web",
    source_type: "local",
    local_path: "/work/web",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "task/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  }) as Repository;

const remoteRepository = (overrides: Partial<RemoteRepository>): RemoteRepository => ({
  provider: "github",
  id: "acme/web",
  owner: "acme",
  name: "web",
  fullName: "acme/web",
  url: "https://github.com/acme/web",
  defaultBranch: "main",
  private: false,
  ...overrides,
});

describe("buildRepositoryRuleCatalog", () => {
  it("deduplicates saved and remote identities while retaining source ordering", () => {
    const options = buildRepositoryRuleCatalog({
      workspaceRepositories: [
        savedRepository({
          provider: "github",
          provider_repo_id: "acme/web",
          provider_owner: "acme",
          provider_name: "web",
          local_path: "/work/web",
        }),
      ],
      localRepositories: [],
      remoteRepositories: [remoteRepository({})],
    });

    expect(options).toHaveLength(1);
    expect(options[0]).toMatchObject({ label: "acme/web", group: "workspace", available: true });
  });

  it("includes plugin repositories and filters by label or identity details", () => {
    const options = buildRepositoryRuleCatalog({
      workspaceRepositories: [],
      localRepositories: [],
      remoteRepositories: [
        remoteRepository({
          provider: "bitbucket",
          id: "42",
          fullName: "platform/api",
          providerHost: "bitbucket.org",
        }),
      ],
      query: "bitbucket",
    });
    expect(options).toHaveLength(1);
    expect(options[0]).toMatchObject({ group: "plugin", label: "platform/api" });
  });

  it("keeps a missing persisted target available for replacement", () => {
    const target = { kind: "local" as const, path: "/old/repository" };
    const options = buildRepositoryRuleCatalog({
      workspaceRepositories: [],
      localRepositories: [],
      remoteRepositories: [],
      unavailableTargets: [{ target, label: "Old repository" }],
    });
    expect(options).toEqual([
      {
        key: repositoryRuleTargetKey(target),
        label: "Old repository",
        group: "unavailable",
        target,
        available: false,
      },
    ]);
  });
});
