import { describe, it, expect, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useDefaultSelectionsEffect } from "./task-create-dialog-effects";
import type { DialogFormState, StoreSelections } from "@/components/task-create-dialog-types";

const PROFILE_DOCKER = "profile-docker";
const PROFILE_LOCAL = "profile-local";
const PROFILE_WORKTREE = "profile-worktree";
const PROFILE_WORKTREE_B = "profile-worktree-b";
const EXECUTOR_DOCKER = "exec-docker";
const REMOTE_URL_A = "github.com/acme/a";
const REMOTE_URL_B = "github.com/acme/b";

type DefaultSelFake = Pick<
  DialogFormState,
  | "agentProfileId"
  | "workflowAgentProfileId"
  | "selectedWorkflowId"
  | "executorId"
  | "executorProfileId"
  | "setAgentProfileId"
  | "setExecutorId"
  | "setExecutorProfileIdFromSeed"
  | "noRepository"
  | "preferLocalExecutor"
  | "repositories"
  | "remoteRepos"
  | "useRemote"
>;

function makeDefaultSelFs(overrides: Partial<DefaultSelFake> = {}): DialogFormState {
  return {
    agentProfileId: "",
    workflowAgentProfileId: "",
    selectedWorkflowId: null,
    executorId: "exec-1",
    executorProfileId: "profile-1",
    setAgentProfileId: vi.fn(),
    setExecutorId: vi.fn(),
    setExecutorProfileIdFromSeed: vi.fn(),
    noRepository: false,
    preferLocalExecutor: false,
    repositories: [],
    remoteRepos: [],
    useRemote: false,
    ...overrides,
  } as unknown as DialogFormState;
}

function makeSel(overrides: Partial<StoreSelections> = {}): StoreSelections {
  return {
    agentProfiles: [],
    compatibleAgentProfiles: [],
    authLoaded: true,
    executors: [],
    workspaceDefaults: null,
    ...overrides,
  };
}

function localExecutor(): StoreSelections["executors"][number] {
  return {
    id: "exec-local",
    type: "local",
    profiles: [{ id: PROFILE_LOCAL, executor_type: "local" }],
  } as unknown as StoreSelections["executors"][number];
}

function dockerExecutor(): StoreSelections["executors"][number] {
  return {
    id: EXECUTOR_DOCKER,
    type: "local_docker",
    profiles: [{ id: PROFILE_DOCKER, executor_type: "local_docker" }],
  } as unknown as StoreSelections["executors"][number];
}

function worktreeExecutor(): StoreSelections["executors"][number] {
  return {
    id: "exec-worktree",
    type: "worktree",
    profiles: [
      { id: PROFILE_WORKTREE, executor_type: "worktree" },
      { id: PROFILE_WORKTREE_B, executor_type: "worktree" },
    ],
  } as unknown as StoreSelections["executors"][number];
}

describe("useDefaultSelectionsEffect - executor profile defaults", () => {
  it("does not let a saved local profile override the worktree default", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "" });
    const local = localExecutor();
    const worktree = worktreeExecutor();
    const sel = makeSel({
      executors: [local, worktree],
      lastUsedExecutorProfileId: PROFILE_LOCAL,
      userSettingsLoaded: true,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_WORKTREE),
    );
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalledWith(PROFILE_LOCAL);
    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(worktree.id));
    expect(fs.setExecutorId).not.toHaveBeenCalledWith(local.id);
  });

  it("defaults repo-backed tasks to the worktree profile when no profile was saved", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "" });
    const local = localExecutor();
    const worktree = worktreeExecutor();
    const sel = makeSel({ executors: [local, worktree] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(worktree.id));
    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_WORKTREE),
    );
  });

  it("defaults repo-less tasks to a local profile because worktree needs a repo", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "", noRepository: true });
    const worktree = worktreeExecutor();
    const local = localExecutor();
    const sel = makeSel({
      executors: [worktree, local],
      lastUsedExecutorProfileId: PROFILE_WORKTREE,
      userSettingsLoaded: true,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL),
    );
  });

  it("defaults explicit local-path tasks to a local profile when no profile was saved", async () => {
    const fs = makeDefaultSelFs({
      executorId: "",
      executorProfileId: "",
      repositories: [{ key: "row-0", localPath: "/workspace/custom", branch: "" }],
    });
    const worktree = worktreeExecutor();
    const local = localExecutor();
    const sel = makeSel({
      executors: [worktree, local],
      lastUsedExecutorProfileId: PROFILE_WORKTREE,
      userSettingsLoaded: true,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL),
    );
  });

  it("ignores a workspace-default worktree executor for explicit local-path tasks", async () => {
    const fs = makeDefaultSelFs({
      executorId: "",
      executorProfileId: "",
      repositories: [{ key: "row-0", localPath: "/workspace/custom", branch: "" }],
    });
    const worktree = worktreeExecutor();
    const local = localExecutor();
    const sel = makeSel({
      executors: [worktree, local],
      workspaceDefaults: {
        default_executor_id: worktree.id,
      } as StoreSelections["workspaceDefaults"],
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    expect(fs.setExecutorId).not.toHaveBeenCalledWith(worktree.id);
    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL),
    );
  });

  it("does not fall back to a worktree profile for explicit local-path tasks", async () => {
    const fs = makeDefaultSelFs({
      executorId: "",
      executorProfileId: "",
      repositories: [{ key: "row-0", localPath: "/workspace/custom", branch: "" }],
    });
    const localWithoutProfiles = { ...localExecutor(), profiles: [] };
    const worktree = worktreeExecutor();
    const docker = dockerExecutor();
    const sel = makeSel({ executors: [worktree, localWithoutProfiles, docker] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_DOCKER),
    );
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalledWith(PROFILE_WORKTREE);
  });
});

// AC-TASKS-RUNNER-SWITCH-004.5a: a task with a stored executor profile seeds
// the picker from that stored value, not from the create-mode "resolve a
// default" autopick.
describe("useDefaultSelectionsEffect - editing task's stored executor profile", () => {
  it("seeds the stored profile instead of resolving a create-mode default", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "" });
    const local = localExecutor();
    const worktree = worktreeExecutor();
    const sel = makeSel({ executors: [local, worktree] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, [], PROFILE_LOCAL));

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL),
    );
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalledWith(PROFILE_WORKTREE);
  });

  it("does not reseed once the picker already carries a value", async () => {
    const fs = makeDefaultSelFs({ executorId: "exec-local", executorProfileId: PROFILE_WORKTREE });
    const sel = makeSel({ executors: [worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, [], PROFILE_LOCAL));

    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalled();
  });

  it("falls back to the create-mode default when the task stores no profile", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "" });
    const worktree = worktreeExecutor();
    const sel = makeSel({ executors: [worktree] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, [], null));

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_WORKTREE),
    );
  });
});

describe("useDefaultSelectionsEffect - executor profile restoration", () => {
  it("defers executor profile fallback until user settings have loaded or settled", async () => {
    const fs = makeDefaultSelFs({ executorProfileId: "", executorId: "" });
    const worktree = worktreeExecutor();
    const selBefore = makeSel({
      executors: [worktree],
      userSettingsLoaded: false,
    });

    const { rerender } = renderHook(({ sel }) => useDefaultSelectionsEffect(fs, true, sel, []), {
      initialProps: { sel: selBefore },
    });

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorId).not.toHaveBeenCalled();
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalled();

    const selAfter = makeSel({
      executors: [worktree],
      userSettingsLoaded: true,
    });
    rerender({ sel: selAfter });

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_WORKTREE),
    );
  });
});

describe("useDefaultSelectionsEffect - executor profile settings restoration", () => {
  it("restores executor profile from backend settings", async () => {
    const fs = makeDefaultSelFs({ executorProfileId: "", executorId: "" });
    const worktree = worktreeExecutor();
    const sel = makeSel({
      executors: [worktree],
      lastUsedExecutorProfileId: PROFILE_WORKTREE_B,
      userSettingsLoaded: true,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_WORKTREE_B),
    );
  });

  it("keeps an explicit local workspace default ahead of a saved worktree profile", async () => {
    const local = localExecutor();
    const worktree = worktreeExecutor();
    const sel = makeSel({
      executors: [local, worktree],
      workspaceDefaults: { default_executor_id: local.id } as StoreSelections["workspaceDefaults"],
      lastUsedExecutorProfileId: PROFILE_WORKTREE_B,
      userSettingsLoaded: true,
    });

    const setExecutorId = vi.fn();
    const setExecutorProfileIdFromSeed = vi.fn();
    const { rerender } = renderHook(
      ({ formState }) => useDefaultSelectionsEffect(formState, true, sel, []),
      {
        initialProps: {
          formState: makeDefaultSelFs({
            executorProfileId: "",
            executorId: "",
            setExecutorId,
            setExecutorProfileIdFromSeed,
          }),
        },
      },
    );

    await waitFor(() => expect(setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL));
    expect(setExecutorProfileIdFromSeed).not.toHaveBeenCalledWith(PROFILE_WORKTREE_B);
    rerender({
      formState: makeDefaultSelFs({
        executorProfileId: PROFILE_LOCAL,
        executorId: "",
        setExecutorId,
        setExecutorProfileIdFromSeed,
      }),
    });

    await waitFor(() => expect(setExecutorId).toHaveBeenCalledWith(local.id));
    expect(setExecutorId).not.toHaveBeenCalledWith(worktree.id);
  });
});

describe("useDefaultSelectionsEffect - launch preferences", () => {
  it("honors a launch-only local preference over the workspace default", async () => {
    const fs = makeDefaultSelFs({
      executorId: "",
      executorProfileId: "",
      noRepository: true,
      preferLocalExecutor: true,
    });
    const docker = dockerExecutor();
    const local = localExecutor();
    const sel = makeSel({
      executors: [docker, local],
      workspaceDefaults: {
        default_executor_id: docker.id,
      } as StoreSelections["workspaceDefaults"],
      userSettingsLoaded: true,
    });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    await waitFor(() =>
      expect(fs.setExecutorProfileIdFromSeed).toHaveBeenCalledWith(PROFILE_LOCAL),
    );
  });
});

describe("useDefaultSelectionsEffect - multi-repo executor selection", () => {
  it("keeps Local Docker selected when 2+ Remote URL rows are set", async () => {
    const fs = makeDefaultSelFs({
      executorId: EXECUTOR_DOCKER,
      executorProfileId: PROFILE_DOCKER,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: REMOTE_URL_A, branch: "", source: "paste" },
        { key: "remote-1", url: REMOTE_URL_B, branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [dockerExecutor(), worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalled();
  });

  it("leaves a worktree profile alone when 2+ Remote rows are set", async () => {
    const fs = makeDefaultSelFs({
      executorId: "exec-worktree",
      executorProfileId: PROFILE_WORKTREE,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: REMOTE_URL_A, branch: "", source: "paste" },
        { key: "remote-1", url: REMOTE_URL_B, branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalled();
  });

  it("does not swap when only a single Remote row is filled", async () => {
    const fs = makeDefaultSelFs({
      executorId: EXECUTOR_DOCKER,
      executorProfileId: PROFILE_DOCKER,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: REMOTE_URL_A, branch: "", source: "paste" },
        { key: "remote-1", url: "", branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [dockerExecutor(), worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorProfileIdFromSeed).not.toHaveBeenCalled();
  });
});
