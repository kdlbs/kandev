import { describe, expect, it, vi, beforeEach } from "vitest";
import {
  autoSelectBranch,
  buildCreateTaskPayload,
  buildRepositoriesPayload,
  findUnresolvedProviderRemote,
  shouldShowTaskTitleField,
  validateCreateInputs,
} from "./task-create-dialog-helpers";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
const STORAGE_KEYS = { LAST_BRANCH: "kandev.dialog.lastBranch" } as const;

beforeEach(() => {
  localStorage.clear();
});

describe("autoSelectBranch", () => {
  const branches = [
    { name: "main", type: "local" as const },
    { name: "feature", type: "local" as const },
  ];

  it("prefers a store-backed branch over a divergent localStorage branch", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("feature"));

    autoSelectBranch(branches, setBranch, { lastUsedBranch: "main" });

    expect(setBranch).toHaveBeenCalledWith("main");
  });

  it("uses the backend last-used branch before settings finish loading", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, {
      lastUsedBranch: "feature",
      userSettingsLoaded: false,
    });

    expect(setBranch).toHaveBeenCalledWith("feature");
  });

  it("uses the backend last-used branch when browser storage is stale", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("deleted"));

    autoSelectBranch(branches, setBranch, {
      lastUsedBranch: "feature",
      userSettingsLoaded: true,
    });

    expect(setBranch).toHaveBeenCalledWith("feature");
  });

  it("defers preferred fallback while user settings are still loading", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: false });

    expect(setBranch).not.toHaveBeenCalled();
  });

  it("falls back to preferred branch after user settings have loaded without a valid last-used branch", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: true });

    expect(setBranch).toHaveBeenCalledWith("main");
  });

  it("ignores a stale localStorage branch after user settings have loaded", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("feature"));

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: true });

    expect(setBranch).toHaveBeenCalledWith("main");
  });

  it("matches remote branch display names for store-backed last-used branch", () => {
    const setBranch = vi.fn();

    autoSelectBranch([{ name: "feature", type: "remote" as const, remote: "origin" }], setBranch, {
      lastUsedBranch: "origin/feature",
      userSettingsLoaded: true,
    });

    expect(setBranch).toHaveBeenCalledWith("origin/feature");
  });

  it("does not pick a branch from an empty branch list", () => {
    const setBranch = vi.fn();

    autoSelectBranch([], setBranch, { lastUsedBranch: "main", userSettingsLoaded: true });

    expect(setBranch).not.toHaveBeenCalled();
  });
});

describe("shouldShowTaskTitleField", () => {
  it.each([
    {
      name: "started edit",
      isCreateMode: false,
      isEditMode: true,
      isTaskStarted: true,
      expected: true,
    },
    {
      name: "new task",
      isCreateMode: true,
      isEditMode: false,
      isTaskStarted: false,
      expected: true,
    },
    {
      name: "create from running task",
      isCreateMode: true,
      isEditMode: false,
      isTaskStarted: true,
      expected: false,
    },
    {
      name: "session",
      isCreateMode: false,
      isEditMode: false,
      isTaskStarted: false,
      expected: false,
    },
  ])(
    "returns $expected for $name mode",
    ({ isCreateMode, isEditMode, isTaskStarted, expected }) => {
      expect(shouldShowTaskTitleField(isCreateMode, isEditMode, isTaskStarted)).toBe(expected);
    },
  );
});

describe("buildRepositoriesPayload — inspected provider repository", () => {
  it("sends the exact inspected clone URL and complete Data Center descriptor", () => {
    const remoteRow = {
      key: "remote-0",
      url: "https://bitbucket.example.test/bitbucket/projects/PLATFORM/repos/web/pull-requests/42",
      remoteUrl: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
      branch: "feature/dc",
      source: "paste",
      provider: "bitbucket",
      providerHost: "https://bitbucket.example.test/bitbucket",
      providerRepoId: "web-42",
      providerOwner: "PLATFORM",
      providerName: "web",
      prNumber: 42,
      prBaseBranch: "main",
      prHeadBranch: "feature/dc",
    } as unknown as TaskRemoteRepoRow;

    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [remoteRow],
      repositories: [],
      discoveredRepositories: [],
    });

    expect(payload).toEqual([
      expect.objectContaining({
        repository_id: "",
        remote_url: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
        provider: "bitbucket",
        provider_host: "https://bitbucket.example.test/bitbucket",
        provider_repo_id: "web-42",
        provider_owner: "PLATFORM",
        provider_name: "web",
        base_branch: "main",
        checkout_branch: "feature/dc",
        pr_number: 42,
      }),
    ]);
    expect(payload[0]).not.toHaveProperty("github_url");
  });
});

describe("findUnresolvedProviderRemote", () => {
  it("blocks a plugin URL until inspection attaches provider identity", () => {
    const row = {
      key: "remote-0",
      url: "https://bitbucket.example.test/projects/TEAM/repos/app",
      branch: "",
      source: "paste",
    } as TaskRemoteRepoRow;
    const matches = (url: string) => url.includes("bitbucket.example.test");

    expect(findUnresolvedProviderRemote([row], matches)).toBe(row);
    expect(findUnresolvedProviderRemote([{ ...row, provider: "bitbucket" }], matches)).toBeNull();
  });
});

describe("auto-title creation helpers", () => {
  const base = {
    workspaceId: "ws-1",
    effectiveWorkflowId: "wf-1",
    repositories: [{ key: "repo-1", repositoryId: "repo-1", branch: "main" }],
    agentProfileId: "agent-1",
    noRepository: false,
  };

  it("requires a prompt instead of a title when auto-title mode is enabled", () => {
    expect(
      validateCreateInputs({ ...base, trimmedTitle: "", trimmedDescription: "", autoTitle: true }),
    ).toBe(false);
    expect(
      validateCreateInputs({
        ...base,
        trimmedTitle: "",
        trimmedDescription: "Describe the work",
        autoTitle: true,
      }),
    ).toBe(true);
  });

  it("omits the manual title and opts into backend provisional naming", () => {
    const payload = buildCreateTaskPayload({
      workspaceId: "ws-1",
      effectiveWorkflowId: "wf-1",
      trimmedTitle: "ignored",
      trimmedDescription: "Fix the login flow",
      repositoriesPayload: [],
      agentProfileId: "agent-1",
      executorId: "executor-1",
      executorProfileId: "profile-1",
      withAgent: true,
      autoTitle: true,
    });

    expect(payload).toMatchObject({ auto_title: true });
    expect(payload).not.toHaveProperty("title");
  });
});
