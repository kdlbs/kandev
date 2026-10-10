import { describe, expect, it, vi } from "vitest";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import { ChangesInlineCommitState } from "./changes-inline-commit-state";

const TASK_CONTEXT = "task-a:session-a";

function localTarget(sha: string, repo = ""): CommitDetailTarget {
  return { source: "local", sha, repo };
}

function files(count: number): Record<string, FileInfo> {
  return Object.fromEntries(
    Array.from({ length: count }, (_, index) => {
      const path = `src/file-${index}.ts`;
      return [
        path,
        {
          path,
          status: "modified",
          staged: false,
          additions: 2,
          deletions: 1,
          diff: `@@ patch for ${path}`,
        } as FileInfo,
      ];
    }),
  );
}

describe("ChangesInlineCommitState bounded snapshots", () => {
  it("bounds expanded historical files and strips patch bodies from row metadata", async () => {
    const request = vi.fn(async () => files(50_000));
    const state = new ChangesInlineCommitState({ contextKey: TASK_CONTEXT, request });
    const target = localTarget("same-sha", "backend");

    await state.expand(target);

    const snapshot = state.get(target);
    expect(snapshot.status).toBe("loaded");
    expect(snapshot.files).toHaveLength(50_000);
    expect(snapshot.files?.[0]).not.toHaveProperty("diff");
    expect(snapshot.files?.[0]).not.toHaveProperty("patch");
  });
});

describe("ChangesInlineCommitState request reuse", () => {
  it("does not fetch details again when a virtual row remounts", async () => {
    const request = vi.fn(async () => files(1));
    const state = new ChangesInlineCommitState({ contextKey: TASK_CONTEXT, request });
    const target = localTarget("commit-1");

    await state.expand(target);
    await state.expand(target);

    expect(request).toHaveBeenCalledTimes(1);
    expect(state.get(target)).toMatchObject({ expanded: true, status: "loaded" });
  });

  it("coalesces concurrent expansion and distinguishes equal SHAs in different repos", async () => {
    const resolveRequests: Array<(value: Record<string, FileInfo>) => void> = [];
    const request = vi.fn(
      () =>
        new Promise<Record<string, FileInfo>>((resolve) => {
          resolveRequests.push(resolve);
        }),
    );
    const state = new ChangesInlineCommitState({ contextKey: TASK_CONTEXT, request });
    const backend = localTarget("same-sha", "backend");
    const frontend = localTarget("same-sha", "frontend");

    const backendRequest = state.expand(backend);
    const duplicateBackendRequest = state.expand(backend);
    const frontendRequest = state.expand(frontend);
    expect(request).toHaveBeenCalledTimes(2);
    for (const resolve of resolveRequests) resolve(files(1));
    await Promise.all([backendRequest, duplicateBackendRequest, frontendRequest]);
  });
});

describe("ChangesInlineCommitState collapsed snapshots", () => {
  it("reuses collapsed snapshots and evicts the least recently used target", async () => {
    const request = vi.fn(async () => files(1));
    const state = new ChangesInlineCommitState({
      contextKey: TASK_CONTEXT,
      request,
      maxCollapsedTargets: 2,
      maxCollapsedFiles: 10,
    });
    const targets = [localTarget("one"), localTarget("two"), localTarget("three")];

    await state.expand(targets[0]);
    state.collapse(targets[0]);
    await state.expand(targets[1]);
    state.collapse(targets[1]);
    await state.expand(targets[2]);
    state.collapse(targets[2]);
    await state.expand(targets[1]);
    await state.expand(targets[0]);

    expect(request).toHaveBeenCalledTimes(4);
    expect(state.get(targets[1]).status).toBe("loaded");
    expect(state.get(targets[0]).status).toBe("loaded");
  });

  it("caches a request that finishes after collapse", async () => {
    let resolveRequest: ((value: Record<string, FileInfo>) => void) | undefined;
    const request = vi.fn(
      () =>
        new Promise<Record<string, FileInfo>>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    const state = new ChangesInlineCommitState({
      contextKey: TASK_CONTEXT,
      request,
      maxCollapsedTargets: 2,
      maxCollapsedFiles: 10,
    });
    const target = localTarget("large");
    const pending = state.expand(target);
    state.collapse(target);
    resolveRequest?.(files(2));
    await pending;

    expect(state.get(target)).toMatchObject({ expanded: false, status: "loaded" });
    await state.expand(target);
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("projects in-flight details even after their commit collapses", async () => {
    let resolveRequest: ((value: Record<string, FileInfo>) => void) | undefined;
    const state = new ChangesInlineCommitState({
      contextKey: TASK_CONTEXT,
      request: () => new Promise((resolve) => (resolveRequest = resolve)),
    });
    const target = localTarget("pending-after-collapse");

    const request = state.expand(target);
    expect(state.getPendingRequestCount()).toBe(1);

    state.collapse(target);
    expect(state.getPendingRequestCount()).toBe(1);

    resolveRequest?.(files(1));
    await request;
    expect(state.getPendingRequestCount()).toBe(0);
  });

  it("evicts a collapsed snapshot that exceeds the file-count limit", async () => {
    const request = vi.fn(async () => files(2));
    const state = new ChangesInlineCommitState({
      contextKey: TASK_CONTEXT,
      request,
      maxCollapsedFiles: 1,
    });
    const target = localTarget("large");

    await state.expand(target);
    state.collapse(target);
    expect(state.get(target).status).toBe("idle");
    await state.expand(target);
    expect(request).toHaveBeenCalledTimes(2);
  });
});

describe("ChangesInlineCommitState retirement and retry", () => {
  it("keeps a request after collapse and ignores completion across A-to-B-to-A context changes", async () => {
    let resolveOldRequest: ((value: Record<string, FileInfo>) => void) | undefined;
    const request = vi.fn(
      () =>
        new Promise<Record<string, FileInfo>>((resolve) => {
          resolveOldRequest = resolve;
        }),
    );
    const state = new ChangesInlineCommitState({ contextKey: "A", request });
    const target = localTarget("commit-1");
    const oldRequest = state.expand(target);
    state.collapse(target);
    state.setContext("B");
    state.setContext("A");
    resolveOldRequest?.(files(1));
    await oldRequest;

    expect(state.get(target)).toMatchObject({ expanded: false, status: "idle" });
  });

  it("requires an explicit retry after a failed source request", async () => {
    const request = vi
      .fn()
      .mockRejectedValueOnce(new Error("provider unavailable"))
      .mockResolvedValueOnce(files(1));
    const state = new ChangesInlineCommitState({ contextKey: TASK_CONTEXT, request });
    const target = {
      source: "github",
      sha: "commit-1",
      workspaceId: "w",
      owner: "o",
      repo: "r",
    } as const;

    await state.expand(target);
    expect(state.get(target)).toMatchObject({ status: "error", error: "provider unavailable" });
    expect(request).toHaveBeenCalledTimes(1);
    await state.retry(target);
    expect(state.get(target)).toMatchObject({ status: "loaded" });
    expect(request).toHaveBeenCalledTimes(2);
    expect(request.mock.calls[0]?.[0].source).toBe("github");
  });
});
