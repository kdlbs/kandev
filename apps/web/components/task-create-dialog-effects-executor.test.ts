import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useDefaultSelectionsEffect } from "./task-create-dialog-effects";
import type { DialogFormState, StoreSelections } from "@/components/task-create-dialog-types";

beforeEach(() => {
  localStorage.clear();
});

const PROFILE_DOCKER = "profile-docker";
const PROFILE_LOCAL = "profile-local";
const PROFILE_WORKTREE = "profile-worktree";

type DefaultSelFake = Pick<
  DialogFormState,
  | "agentProfileId"
  | "workflowAgentProfileId"
  | "selectedWorkflowId"
  | "executorId"
  | "executorProfileId"
  | "setAgentProfileId"
  | "setExecutorId"
  | "setExecutorProfileId"
  | "noRepository"
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
    setExecutorProfileId: vi.fn(),
    noRepository: false,
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
    id: "exec-docker",
    type: "local_docker",
    profiles: [{ id: PROFILE_DOCKER, executor_type: "local_docker" }],
  } as unknown as StoreSelections["executors"][number];
}

function worktreeExecutor(): StoreSelections["executors"][number] {
  return {
    id: "exec-worktree",
    type: "worktree",
    profiles: [{ id: PROFILE_WORKTREE, executor_type: "worktree" }],
  } as unknown as StoreSelections["executors"][number];
}

describe("useDefaultSelectionsEffect - executor profile defaults", () => {
  it("defaults repo-backed tasks to the worktree profile when no profile was saved", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "" });
    const local = localExecutor();
    const worktree = worktreeExecutor();
    const sel = makeSel({ executors: [local, worktree] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(worktree.id));
    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_WORKTREE));
  });

  it("defaults repo-less tasks to a local profile because worktree needs a repo", async () => {
    const fs = makeDefaultSelFs({ executorId: "", executorProfileId: "", noRepository: true });
    const worktree = worktreeExecutor();
    const local = localExecutor();
    const sel = makeSel({ executors: [worktree, local] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_LOCAL));
  });

  it("defaults explicit local-path tasks to a local profile when no profile was saved", async () => {
    const fs = makeDefaultSelFs({
      executorId: "",
      executorProfileId: "",
      repositories: [{ key: "row-0", localPath: "/workspace/custom", branch: "" }],
    });
    const worktree = worktreeExecutor();
    const local = localExecutor();
    const sel = makeSel({ executors: [worktree, local] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorId).toHaveBeenCalledWith(local.id));
    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_LOCAL));
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
    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_LOCAL));
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

    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_DOCKER));
    expect(fs.setExecutorProfileId).not.toHaveBeenCalledWith(PROFILE_WORKTREE);
  });
});

describe("useDefaultSelectionsEffect - multi-repo guard counts Remote rows", () => {
  it("swaps a non-worktree profile to worktree when 2+ Remote URL rows are set", async () => {
    // Regression: the guard used to count only workspace/local rows, so 2
    // Remote rows slipped past it with an already-selected non-worktree profile.
    const fs = makeDefaultSelFs({
      executorId: "exec-docker",
      executorProfileId: PROFILE_DOCKER,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: "github.com/acme/a", branch: "", source: "paste" },
        { key: "remote-1", url: "github.com/acme/b", branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [dockerExecutor(), worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await waitFor(() => expect(fs.setExecutorProfileId).toHaveBeenCalledWith(PROFILE_WORKTREE));
  });

  it("leaves a worktree profile alone when 2+ Remote rows are set", async () => {
    const fs = makeDefaultSelFs({
      executorId: "exec-worktree",
      executorProfileId: PROFILE_WORKTREE,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: "github.com/acme/a", branch: "", source: "paste" },
        { key: "remote-1", url: "github.com/acme/b", branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorProfileId).not.toHaveBeenCalled();
  });

  it("does not swap when only a single Remote row is filled", async () => {
    const fs = makeDefaultSelFs({
      executorId: "exec-docker",
      executorProfileId: PROFILE_DOCKER,
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: "github.com/acme/a", branch: "", source: "paste" },
        { key: "remote-1", url: "", branch: "", source: "paste" },
      ] as DialogFormState["remoteRepos"],
    });
    const sel = makeSel({ executors: [dockerExecutor(), worktreeExecutor()] });

    renderHook(() => useDefaultSelectionsEffect(fs, true, sel, []));

    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(fs.setExecutorProfileId).not.toHaveBeenCalled();
  });
});
