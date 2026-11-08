import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GitLabMRFeedback } from "@/lib/types/gitlab";

const api = vi.hoisted(() => ({
  getMRFeedback: vi.fn(),
  getMRFiles: vi.fn(),
  getMRCommits: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => api);

import { useMRFeedback } from "./use-mr-feedback";

const GITLAB_HOST = "https://gitlab.example";
const WORKSPACE_ID = "ws";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function rejected(message: string) {
  return Promise.reject(new Error(message));
}

function feedback(title: string): GitLabMRFeedback {
  return {
    mr: { title } as GitLabMRFeedback["mr"],
    approvals: [],
    discussions: [],
    pipelines: [],
    has_issues: false,
  };
}

describe("useMRFeedback identity isolation", () => {
  beforeEach(() => vi.clearAllMocks());

  it("clears A while B loads and ignores delayed A responses", async () => {
    const a = deferred<GitLabMRFeedback>();
    const b = deferred<GitLabMRFeedback>();
    api.getMRFeedback.mockImplementation(({ project }: { project: string }) =>
      project === "group/a" ? a.promise : b.promise,
    );
    api.getMRFiles.mockResolvedValue({ files: [] });
    api.getMRCommits.mockResolvedValue({ commits: [] });

    const { result, rerender } = renderHook(
      ({ project }) => useMRFeedback(WORKSPACE_ID, project, 1, GITLAB_HOST),
      { initialProps: { project: "group/a" } },
    );
    rerender({ project: "group/b" });

    expect(result.current.feedback).toBeNull();
    expect(result.current.loading).toBe(true);
    act(() => a.resolve(feedback("A")));
    await Promise.resolve();
    expect(result.current.feedback).toBeNull();

    act(() => b.resolve(feedback("B")));
    await waitFor(() => expect(result.current.feedback?.mr.title).toBe("B"));
    expect(api.getMRFeedback).toHaveBeenLastCalledWith({
      workspaceId: "ws",
      project: "group/b",
      iid: 1,
      host: GITLAB_HOST,
    });
  });

  it("retains last-known-good data while a same-identity refresh fails", async () => {
    api.getMRFeedback.mockResolvedValueOnce(feedback("Current"));
    api.getMRFiles.mockResolvedValueOnce({ files: [{ old_path: "a", new_path: "a" }] });
    api.getMRCommits.mockResolvedValueOnce({ commits: [{ id: "sha" }] });
    const { result } = renderHook(() => useMRFeedback(WORKSPACE_ID, "group/a", 1, GITLAB_HOST));
    await waitFor(() => expect(result.current.feedback?.mr.title).toBe("Current"));

    api.getMRFeedback.mockImplementationOnce(() => rejected("refresh failed"));
    api.getMRFiles.mockImplementationOnce(() => rejected("files failed"));
    api.getMRCommits.mockImplementationOnce(() => rejected("commits failed"));
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.error).toContain("refresh failed"));

    expect(result.current.feedback?.mr.title).toBe("Current");
    expect(result.current.files).toHaveLength(1);
    expect(result.current.commits).toHaveLength(1);
  });

  it("keeps successful feedback when files fail and reports the partial error", async () => {
    api.getMRFeedback.mockResolvedValue(feedback("Review"));
    api.getMRFiles.mockImplementation(() => rejected("files unavailable"));
    api.getMRCommits.mockResolvedValue({ commits: [{ id: "sha" }] });
    const { result } = renderHook(() => useMRFeedback(WORKSPACE_ID, "group/a", 1, GITLAB_HOST));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.feedback?.mr.title).toBe("Review");
    expect(result.current.commits).toHaveLength(1);
    expect(result.current.error).toContain("files unavailable");
  });
});

describe("useMRFeedback reload", () => {
  beforeEach(() => vi.clearAllMocks());

  async function mounted() {
    api.getMRFeedback.mockResolvedValue(feedback("first"));
    api.getMRFiles.mockResolvedValue({ files: [] });
    api.getMRCommits.mockResolvedValue({ commits: [] });
    const hook = renderHook(() => useMRFeedback(WORKSPACE_ID, "group/a", 1, GITLAB_HOST));
    await waitFor(() => expect(hook.result.current.feedback?.mr.title).toBe("first"));
    return hook;
  }

  it("resolves ok with the reloaded MR and applies it", async () => {
    const { result } = await mounted();
    api.getMRFeedback.mockResolvedValue(feedback("second"));
    let outcome: Awaited<ReturnType<typeof result.current.reload>> | undefined;
    await act(async () => {
      outcome = await result.current.reload();
    });
    expect(outcome).toEqual({ outcome: "ok", mr: { title: "second" } });
    expect(result.current.feedback?.mr.title).toBe("second");
  });

  it("is failed when the feedback request fails and keeps the previous feedback", async () => {
    const { result } = await mounted();
    api.getMRFeedback.mockImplementation(() => rejected("feedback down"));
    let outcome: Awaited<ReturnType<typeof result.current.reload>> | undefined;
    await act(async () => {
      outcome = await result.current.reload();
    });
    expect(outcome).toEqual({ outcome: "failed" });
    expect(result.current.feedback?.mr.title).toBe("first");
    expect(result.current.error).toContain("feedback down");
  });

  it("is still ok when only files or commits fail", async () => {
    const { result } = await mounted();
    api.getMRFeedback.mockResolvedValue(feedback("second"));
    api.getMRFiles.mockImplementation(() => rejected("files down"));
    let outcome: Awaited<ReturnType<typeof result.current.reload>> | undefined;
    await act(async () => {
      outcome = await result.current.reload();
    });
    expect(outcome?.outcome).toBe("ok");
    expect(result.current.error).toContain("files down");
  });

  it("is superseded by a newer reload and dispatches nothing for the older one", async () => {
    const { result } = await mounted();
    const older = deferred<GitLabMRFeedback>();
    api.getMRFeedback.mockReturnValueOnce(older.promise);
    let olderOutcome: Promise<Awaited<ReturnType<typeof result.current.reload>>> | undefined;
    act(() => {
      olderOutcome = result.current.reload();
    });
    api.getMRFeedback.mockResolvedValue(feedback("newer"));
    await act(async () => {
      await result.current.reload();
    });
    await act(async () => {
      older.resolve(feedback("older"));
      expect(await olderOutcome).toEqual({ outcome: "superseded" });
    });
    expect(result.current.feedback?.mr.title).toBe("newer");
  });

  it("is superseded by refresh() and by unmount", async () => {
    const { result, unmount } = await mounted();
    const slow = deferred<GitLabMRFeedback>();
    api.getMRFeedback.mockReturnValueOnce(slow.promise);
    let pending: Promise<Awaited<ReturnType<typeof result.current.reload>>> | undefined;
    act(() => {
      pending = result.current.reload();
    });
    api.getMRFeedback.mockResolvedValue(feedback("refreshed"));
    act(() => result.current.refresh());
    await act(async () => {
      slow.resolve(feedback("late"));
      expect(await pending).toEqual({ outcome: "superseded" });
    });
    await waitFor(() => expect(result.current.feedback?.mr.title).toBe("refreshed"));

    const last = deferred<GitLabMRFeedback>();
    api.getMRFeedback.mockReturnValueOnce(last.promise);
    act(() => {
      pending = result.current.reload();
    });
    unmount();
    last.resolve(feedback("after unmount"));
    expect(await pending).toEqual({ outcome: "superseded" });
  });
});
