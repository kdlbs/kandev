import { describe, expect, it, vi } from "vitest";
import type { Repository } from "@/lib/types/http";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import {
  ProjectRepositoryResolutionError,
  projectRemoteRepositoryKey,
  resolveAgentProjectRepositories,
} from "./repository-resolution";

function remote(id: string): RemoteRepository {
  return {
    provider: "forge",
    id,
    owner: "team",
    name: id,
    fullName: `team/${id}`,
    url: `https://forge.example.test/team/${id}.git`,
    providerHost: "https://forge.example.test",
    providerScope: "workspace-a",
    defaultBranch: "main",
    private: true,
  };
}

function record(id: string, source: RemoteRepository = remote(id)): Repository {
  return {
    id: id as Repository["id"],
    workspace_id: "ws-1" as Repository["workspace_id"],
    name: source.fullName,
    source_type: "provider",
    local_path: "",
    provider: source.provider,
    provider_repo_id: source.id,
    provider_host: source.providerHost,
    provider_scope: source.providerScope,
    provider_owner: source.owner,
    provider_name: source.name,
    remote_url: source.url,
    default_branch: source.defaultBranch,
    worktree_branch_prefix: "",
    worktree_branch_template: "",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
  };
}

describe("resolveAgentProjectRepositories", () => {
  it("keeps a successful import cached when the next import fails", async () => {
    const first = remote("repo-1");
    const second = remote("repo-2");
    const cache = new Map<string, Repository>();
    const create = vi.fn(async (selected: RemoteRepository) => {
      if (selected.id === "repo-2") throw new Error("provider temporarily unavailable");
      return record(selected.id, selected);
    });

    await expect(
      resolveAgentProjectRepositories({
        workspaceId: "ws-1",
        selected: [first, second],
        existing: [],
        cache,
        create,
      }),
    ).rejects.toThrow("provider temporarily unavailable");
    expect(cache.get(projectRemoteRepositoryKey(first))?.id).toBe("repo-1");

    create.mockImplementation(async (selected) => record(selected.id, selected));
    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [first, second],
      existing: [],
      cache,
      create,
    });
    expect(resolved.map((repository) => repository.id)).toEqual(["repo-1", "repo-2"]);
    expect(create).toHaveBeenCalledTimes(3);
  });
});

describe("resolveAgentProjectRepositories provider identity", () => {
  it("reuses an existing record only when full provider identity matches", async () => {
    const selected = remote("same-id");
    const wrongHost = record("wrong-host", {
      ...selected,
      providerHost: "https://other.example.test",
    });
    const matching = record("matching", selected);
    const create = vi.fn(async (repository: RemoteRepository) => record("created", repository));
    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [selected],
      existing: [wrongHost, matching],
      cache: new Map(),
      create,
    });

    expect(resolved).toEqual([matching]);
    expect(create).not.toHaveBeenCalled();
  });

  it("reuses a legacy record with an empty provider ID when host and name prove identity", async () => {
    const selected = { ...remote("legacy-id"), providerScope: undefined };
    const legacy = record("legacy-record", selected);
    legacy.provider_repo_id = "";
    legacy.provider_host = "";
    legacy.provider_scope = "";
    const create = vi.fn(async (repository: RemoteRepository) => record("created", repository));

    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [selected],
      existing: [legacy],
      cache: new Map(),
      create,
    });

    expect(resolved).toEqual([legacy]);
    expect(create).not.toHaveBeenCalled();
  });

  it("does not infer a matching self-hosted record from the repo ID when host is omitted", async () => {
    const selected = { ...remote("same-id"), providerHost: undefined };
    const otherHost = record("other-host", {
      ...selected,
      url: "https://other.example.test/team/same-id.git",
    });
    otherHost.provider_host = "https://other.example.test";
    otherHost.remote_url = "https://other.example.test/team/same-id.git";
    const create = vi.fn(async (repository: RemoteRepository) => record("created", repository));

    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [selected],
      existing: [otherHost],
      cache: new Map(),
      create,
    });

    expect(resolved[0]?.id).toBe("created");
    expect(create).toHaveBeenCalledOnce();
  });

  it("does not reuse a same-host record from a different provider scope", async () => {
    const selected = remote("same-id");
    const otherScope = record("other-scope", { ...selected, providerScope: "workspace-b" });
    const create = vi.fn(async (repository: RemoteRepository) => record("created", repository));

    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [selected],
      existing: [otherScope],
      cache: new Map(),
      create,
    });

    expect(resolved[0]?.id).toBe("created");
    expect(create).toHaveBeenCalledOnce();
  });
});

describe("resolveAgentProjectRepositories verified import", () => {
  it("preserves a nondefault host port while comparing provider identity", async () => {
    const selected = {
      ...remote("same-id"),
      providerHost: "https://forge.example.test:8443",
      url: "https://forge.example.test:8443/team/same-id.git",
    };
    const matching = record("matching-port", selected);
    const wrongPort = record("wrong-port", {
      ...selected,
      providerHost: "https://forge.example.test:9443",
      url: "https://forge.example.test:9443/team/same-id.git",
    });
    const create = vi.fn(async (repository: RemoteRepository) => record("created", repository));

    const resolved = await resolveAgentProjectRepositories({
      workspaceId: "ws-1",
      selected: [selected],
      existing: [wrongPort, matching],
      cache: new Map(),
      create,
    });

    expect(resolved).toEqual([matching]);
    expect(create).not.toHaveBeenCalled();
  });

  it("rejects a created repository whose provider identity does not match the selection", async () => {
    const selected = remote("selected-id");
    const create = vi.fn(async () => {
      const wrong = record("wrong-record", { ...selected, id: "different-id" });
      return wrong;
    });
    const cache = new Map<string, Repository>();

    await expect(
      resolveAgentProjectRepositories({
        workspaceId: "ws-1",
        selected: [selected],
        existing: [],
        cache,
        create,
      }),
    ).rejects.toThrow(ProjectRepositoryResolutionError);
    expect(cache.size).toBe(0);
  });
});
