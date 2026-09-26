import { describe, expect, it } from "vitest";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import { repositoryId } from "@/lib/types/ids";
import {
  areRepositorySecretBindingsEqual,
  cloneRepository,
  isRepositoryDirty,
  mergeSavedRepositoryDraft,
  persistedRepositoryItems,
  type RepositoryWithScripts,
} from "./workspace-repositories-dirty";

const script = (overrides: Partial<RepositoryScript> = {}): RepositoryScript => ({
  id: "script-1",
  repository_id: repositoryId("repo-1"),
  name: "setup",
  command: "echo setup",
  position: 0,
  created_at: "",
  updated_at: "",
  ...overrides,
});

const repository = (overrides: Partial<Repository> = {}): RepositoryWithScripts =>
  ({
    id: repositoryId("repo-1"),
    workspace_id: "workspace-1",
    name: "repo",
    source_type: "local",
    local_path: "/tmp/repo",
    provider: "",
    provider_repo_id: "",
    provider_host: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    worktree_branch_template: "feature/{title}-{suffix}",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    scripts: [],
    ...overrides,
  }) as RepositoryWithScripts;

describe("repository secret binding draft state", () => {
  it("compares bindings independent of row order", () => {
    const left = repository({
      secret_bindings: [
        { key: "NPM_TOKEN", secret_id: "secret-npm" },
        { key: "API_TOKEN", secret_id: "secret-api" },
      ],
    });
    const right = repository({
      secret_bindings: [
        { key: "API_TOKEN", secret_id: "secret-api" },
        { key: "NPM_TOKEN", secret_id: "secret-npm" },
      ],
    });

    expect(areRepositorySecretBindingsEqual(left, right)).toBe(true);
    expect(isRepositoryDirty(left, right)).toBe(false);
  });

  it("deep-clones bindings so draft edits do not mutate the saved row", () => {
    const source = repository({ secret_bindings: [{ key: "TOKEN", secret_id: "secret-1" }] });
    const cloned = cloneRepository(source);

    cloned.secret_bindings![0]!.key = "CHANGED";

    expect(source.secret_bindings![0]!.key).toBe("TOKEN");
  });

  it("preserves a locally edited binding when the save response is merged", () => {
    const saved = repository({ secret_bindings: [{ key: "TOKEN", secret_id: "secret-old" }] });
    const submitted = repository({ secret_bindings: [{ key: "TOKEN", secret_id: "secret-old" }] });
    const current = repository({ secret_bindings: [{ key: "TOKEN", secret_id: "secret-new" }] });

    const merged = mergeSavedRepositoryDraft(current, submitted, saved);

    expect(merged.secret_bindings).toEqual([{ key: "TOKEN", secret_id: "secret-new" }]);
  });

  it("detects explicit clear as dirty", () => {
    const saved = repository({ secret_bindings: [{ key: "TOKEN", secret_id: "secret-1" }] });
    const draft = repository({ secret_bindings: [] });

    expect(isRepositoryDirty(draft, saved)).toBe(true);
  });

  it("clones scripts alongside repository bindings", () => {
    const source = repository({ scripts: [script()] });
    const cloned = cloneRepository(source);

    cloned.scripts[0]!.command = "changed";

    expect(source.scripts[0]!.command).toBe("echo setup");
  });

  it("keeps temporary repository drafts out of persisted repository lists", () => {
    const persisted = repository();
    const draft = repository({ id: repositoryId("temp-repo-1") });

    expect(persistedRepositoryItems([persisted, draft])).toEqual([persisted]);
  });
});
