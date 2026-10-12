import { createElement, StrictMode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const preflightWorkspaceUpload = vi.fn();
const uploadWorkspaceFile = vi.fn();

vi.mock("@/lib/api/domains/workspace-file-api", () => ({
  preflightWorkspaceUpload: (...args: unknown[]) => preflightWorkspaceUpload(...args),
  uploadWorkspaceFile: (...args: unknown[]) => uploadWorkspaceFile(...args),
}));

import { useFileUpload, type ConflictChoice } from "./use-file-upload";

const A_TXT = "a.txt";
const B_TXT = "b.txt";
const C_TXT = "c.txt";
const FIXTURES = "fixtures";
const INVALID_PATH = "../invalid.txt";

function file(name: string): File {
  return new File(["bytes"], name);
}

beforeEach(() => {
  preflightWorkspaceUpload.mockReset();
  uploadWorkspaceFile.mockReset();
  preflightWorkspaceUpload.mockResolvedValue([]);
  uploadWorkspaceFile.mockImplementation(
    async ({ dir, relativePath }: { dir: string; relativePath: string }) => ({
      path: dir ? `${dir}/${relativePath}` : relativePath,
      size_bytes: 5,
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useFileUpload with no conflicts", () => {
  it("uploads every file without prompting", async () => {
    const { result } = renderHook(() => useFileUpload("sess-1"));

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    await act(async () => {
      outcome = await result.current.uploadFiles(FIXTURES, [file(A_TXT), file(B_TXT)]);
    });

    expect(result.current.conflicts).toBeNull();
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(2);
    expect(outcome?.uploaded.map((u) => u.path)).toEqual([
      `${FIXTURES}/${A_TXT}`,
      `${FIXTURES}/${B_TXT}`,
    ]);
    expect(outcome?.cancelled).toBe(false);
    await waitFor(() =>
      expect(result.current.uploads.every((u) => u.status === "ready")).toBe(true),
    );
  });

  it("sends no resolution when nothing conflicts, so a silent overwrite is impossible", async () => {
    const { result } = renderHook(() => useFileUpload("sess-1"));
    await act(async () => {
      await result.current.uploadFiles("", [file(A_TXT)]);
    });

    expect(uploadWorkspaceFile.mock.calls[0][0]).toMatchObject({ resolution: undefined });
  });

  it("does nothing without an active session", async () => {
    const { result } = renderHook(() => useFileUpload(null));
    await act(async () => {
      await result.current.uploadFiles("", [file(A_TXT)]);
    });

    expect(preflightWorkspaceUpload).not.toHaveBeenCalled();
    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
  });

  it("reports skipped selections instead of dropping them", async () => {
    const { result } = renderHook(() => useFileUpload("sess-1"));
    const invalid = new File(["bytes"], "evil.txt");
    Object.defineProperty(invalid, "webkitRelativePath", { value: "../evil.txt" });

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    await act(async () => {
      outcome = await result.current.uploadFiles("", [file(A_TXT), invalid]);
    });

    expect(outcome?.skipped).toEqual(["../evil.txt"]);
    expect(outcome?.failed).toBe(1);
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1);
    expect(result.current.uploads.find((item) => item.relativePath === "../evil.txt")?.status).toBe(
      "failed",
    );
  });

  it("does not overlap a second batch with an active upload", async () => {
    let releaseFirst!: () => void;
    uploadWorkspaceFile.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          releaseFirst = () => resolve({ path: A_TXT, size_bytes: 5 });
        }),
    );
    const { result } = renderHook(() => useFileUpload("sess-1"));

    let first!: Promise<Awaited<ReturnType<typeof result.current.uploadFiles>>>;
    await act(async () => {
      first = result.current.uploadFiles("", [file(A_TXT)]);
      await Promise.resolve();
    });
    const second = await result.current.uploadFiles("", [file(B_TXT)]);
    expect(second.failed).toBe(1);
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1);

    await act(async () => {
      releaseFirst();
      await first;
    });
  });
});

describe("useFileUpload conflict flow", () => {
  it("uploads nothing until the conflicts are resolved", async () => {
    preflightWorkspaceUpload.mockResolvedValue([{ path: `${FIXTURES}/${A_TXT}`, is_dir: false }]);
    const { result } = renderHook(() => useFileUpload("sess-1"));

    act(() => {
      void result.current.uploadFiles(FIXTURES, [file(A_TXT), file(B_TXT)]);
    });

    await waitFor(() => expect(result.current.conflicts).not.toBeNull());
    expect(result.current.conflicts?.conflicts).toEqual([
      { path: `${FIXTURES}/${A_TXT}`, is_dir: false },
    ]);
    // The whole batch is parked, including the file that did not conflict.
    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
  });

  it("cancelling writes nothing at all, including unconflicted files", async () => {
    preflightWorkspaceUpload.mockResolvedValue([{ path: `${FIXTURES}/${A_TXT}`, is_dir: false }]);
    const { result } = renderHook(() => useFileUpload("sess-1"));

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    act(() => {
      void result.current
        .uploadFiles(FIXTURES, [file(A_TXT), file(B_TXT)])
        .then((r) => (outcome = r));
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());

    await act(async () => {
      result.current.cancelConflicts();
    });

    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
    await waitFor(() => expect(outcome?.cancelled).toBe(true));
    expect(result.current.uploads).toEqual([]);
  });

  it("applies a per-file resolution and omits skipped files", async () => {
    preflightWorkspaceUpload.mockResolvedValue([
      { path: `${FIXTURES}/${A_TXT}`, is_dir: false },
      { path: `${FIXTURES}/${B_TXT}`, is_dir: false },
    ]);
    const { result } = renderHook(() => useFileUpload("sess-1"));

    act(() => {
      void result.current.uploadFiles(FIXTURES, [file(A_TXT), file(B_TXT), file(C_TXT)]);
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());

    const choices = new Map<string, ConflictChoice>([
      [`${FIXTURES}/${A_TXT}`, "replace"],
      [`${FIXTURES}/${B_TXT}`, "skip"],
    ]);
    await act(async () => {
      await result.current.resolveConflicts(choices);
    });

    const uploadedPaths = uploadWorkspaceFile.mock.calls.map((c) => c[0].relativePath);
    expect(uploadedPaths).toEqual([A_TXT, C_TXT]);
    expect(uploadWorkspaceFile.mock.calls[0][0].resolution).toBe("replace");
    // The unconflicted file carries no resolution.
    expect(uploadWorkspaceFile.mock.calls[1][0].resolution).toBeUndefined();
  });
});

describe("useFileUpload failure isolation", () => {
  it("one failing file does not stop the others", async () => {
    uploadWorkspaceFile.mockImplementation(async ({ relativePath }: { relativePath: string }) => {
      if (relativePath === B_TXT) throw new Error("upload failed (413)");
      return { path: relativePath, size_bytes: 5 };
    });
    const { result } = renderHook(() => useFileUpload("sess-1"));

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    await act(async () => {
      outcome = await result.current.uploadFiles("", [file(A_TXT), file(B_TXT), file(C_TXT)]);
    });

    expect(outcome?.uploaded.map((u) => u.path)).toEqual([A_TXT, C_TXT]);
    expect(outcome?.failed).toBe(1);
    const failedItem = result.current.uploads.find((u) => u.relativePath === B_TXT);
    expect(failedItem?.status).toBe("failed");
    expect(failedItem?.error).toContain("413");
  });

  it("records the server-reported path after a rename", async () => {
    preflightWorkspaceUpload.mockResolvedValue([{ path: A_TXT, is_dir: false }]);
    uploadWorkspaceFile.mockResolvedValue({
      path: "a-1.txt",
      size_bytes: 5,
      resolution_applied: "keep_both",
    });
    const { result } = renderHook(() => useFileUpload("sess-1"));

    act(() => {
      void result.current.uploadFiles("", [file(A_TXT)]);
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());

    await act(async () => {
      await result.current.resolveConflicts(new Map([[A_TXT, "keep_both"]]));
    });

    await waitFor(() => expect(result.current.uploads[0]?.writtenPath).toBe("a-1.txt"));
  });

  it("marks every file failed when the preflight itself fails", async () => {
    preflightWorkspaceUpload.mockRejectedValue(new Error("workspace not ready"));
    const { result } = renderHook(() => useFileUpload("sess-1"));

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    await act(async () => {
      outcome = await result.current.uploadFiles("", [file(A_TXT), file(B_TXT)]);
    });

    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
    expect(outcome?.failed).toBe(2);
    expect(result.current.uploads.every((u) => u.status === "failed")).toBe(true);
  });

  it("cancels a parked batch when the session changes", async () => {
    preflightWorkspaceUpload.mockResolvedValue([{ path: `${FIXTURES}/${A_TXT}`, is_dir: false }]);
    const { result, rerender } = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });

    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    act(() => {
      void result.current.uploadFiles(FIXTURES, [file(A_TXT)]).then((value) => (outcome = value));
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());

    rerender({ sessionId: "sess-2" });

    await waitFor(() => expect(outcome?.cancelled).toBe(true));
    expect(result.current.conflicts).toBeNull();
    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
  });

  it("stops an active batch when the session changes", async () => {
    let releaseFirst!: () => void;
    uploadWorkspaceFile.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          releaseFirst = () => resolve({ path: A_TXT, size_bytes: 5 });
        }),
    );
    const { result, rerender } = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });

    let outcome!: Promise<Awaited<ReturnType<typeof result.current.uploadFiles>>>;
    await act(async () => {
      outcome = result.current.uploadFiles("", [file(A_TXT), file(B_TXT)]);
      await waitFor(() => expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1));
    });

    rerender({ sessionId: "sess-2" });
    await act(async () => {
      releaseFirst();
      await outcome;
    });

    await expect(outcome).resolves.toMatchObject({ cancelled: true });
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1);
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function skippedFile() {
  const invalid = file("invalid.txt");
  Object.defineProperty(invalid, "webkitRelativePath", { value: INVALID_PATH });
  return invalid;
}

// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.7, AC-UI-WORKSPACE-FILE-TRANSFER-003.8
// @covers AC-UI-WORKSPACE-FILE-TRANSFER-003.9, AC-UI-WORKSPACE-FILE-TRANSFER-003.10
describe("useFileUpload owner lifetime", () => {
  it("settles a parked batch on owner unmount without uploading any selected file", async () => {
    preflightWorkspaceUpload.mockResolvedValue([{ path: A_TXT, is_dir: false }]);
    const { result, unmount } = renderHook(() => useFileUpload("sess-1"));
    let outcome: Awaited<ReturnType<typeof result.current.uploadFiles>> | undefined;
    let caller!: ReturnType<typeof result.current.uploadFiles>;
    act(() => {
      caller = result.current.uploadFiles("", [file(A_TXT), file(B_TXT), skippedFile()]);
      void caller.then((value) => (outcome = value));
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());
    await act(async () => unmount());
    // Cleanup must settle without a remaining dialog action or network request.
    try {
      expect(outcome).toEqual({
        uploaded: [],
        cancelled: true,
        failed: 0,
        skipped: [INVALID_PATH],
      });
      expect(uploadWorkspaceFile).not.toHaveBeenCalled();
    } finally {
      if (!outcome) result.current.cancelConflicts();
      await caller;
    }
  });

  it.each([
    ["unmount", "clear"],
    ["unmount", "conflict"],
    ["unmount", "failure"],
    ["session", "clear"],
    ["session", "conflict"],
    ["session", "failure"],
  ])("retires deferred preflight after %s with %s response", async (retire, response) => {
    const transport = deferred<Array<{ path: string; is_dir: boolean }>>();
    preflightWorkspaceUpload.mockReturnValueOnce(transport.promise);
    const { result, unmount, rerender } = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });
    let caller!: ReturnType<typeof result.current.uploadFiles>;
    let settled = false;
    act(() => {
      caller = result.current.uploadFiles("", [file(A_TXT), skippedFile()]);
      void caller.then(() => (settled = true));
    });
    if (retire === "unmount") unmount();
    else rerender({ sessionId: "sess-2" });
    expect(settled).toBe(false);
    await act(async () => {
      if (response === "failure") transport.reject(new Error("late preflight failure"));
      else transport.resolve(response === "conflict" ? [{ path: A_TXT, is_dir: false }] : []);
      await transport.promise.catch(() => undefined);
    });
    try {
      expect(settled).toBe(true);
    } finally {
      if (!settled) result.current.cancelConflicts();
      await caller;
    }
    await expect(caller).resolves.toMatchObject({
      cancelled: true,
      failed: 0,
      skipped: [INVALID_PATH],
    });
    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
    if (retire === "session") {
      expect(result.current.conflicts).toBeNull();
      expect(result.current.uploads).toEqual([]);
    }
  });
});

describe("useFileUpload active retirement", () => {
  it.each([
    ["unmount", false, false],
    ["unmount", false, true],
    ["unmount", true, false],
    ["unmount", true, true],
    ["session", false, false],
    ["session", false, true],
    ["session", true, false],
    ["session", true, true],
  ])("stops uploads after %s (conflicts %s, failure %s)", async (retire, conflicting, failure) => {
    const transport = deferred<{ path: string; size_bytes: number }>();
    uploadWorkspaceFile.mockReturnValueOnce(transport.promise);
    if (conflicting) preflightWorkspaceUpload.mockResolvedValue([{ path: A_TXT, is_dir: false }]);
    const { result, rerender, unmount } = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });
    let caller!: ReturnType<typeof result.current.uploadFiles>;
    let resolution: Promise<void> | undefined;
    let settled = false;
    act(() => {
      caller = result.current.uploadFiles("", [file(A_TXT), file(B_TXT), skippedFile()]);
      void caller.then(() => (settled = true));
    });
    if (conflicting) {
      await waitFor(() => expect(result.current.conflicts).not.toBeNull());
      act(() => {
        resolution = result.current.resolveConflicts(new Map([[A_TXT, "keep_both"]]));
      });
    }
    await waitFor(() => expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1));
    if (retire === "unmount") unmount();
    else rerender({ sessionId: "sess-2" });
    expect(settled).toBe(false);
    await act(async () => {
      if (failure) transport.reject(new Error("late upload failure"));
      else transport.resolve({ path: "a-1.txt", size_bytes: 5 });
      await caller;
      await resolution;
    });
    await expect(caller).resolves.toEqual({
      uploaded: failure ? [] : [{ path: "a-1.txt", size_bytes: 5 }],
      cancelled: true,
      failed: failure ? 2 : 1,
      skipped: [INVALID_PATH],
    });
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1);
    if (retire === "session") expect(result.current.uploads).toEqual([]);
  });
});

describe("useFileUpload lifetime isolation", () => {
  it("rejects retained callbacks after returning to the same session", async () => {
    const { result, rerender } = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });
    const old = result.current;
    rerender({ sessionId: "sess-2" });
    rerender({ sessionId: "sess-1" });
    preflightWorkspaceUpload.mockResolvedValue([{ path: B_TXT, is_dir: false }]);
    let caller!: ReturnType<typeof result.current.uploadFiles>;
    act(() => {
      caller = result.current.uploadFiles("", [file(B_TXT)]);
    });
    await waitFor(() => expect(result.current.conflicts).not.toBeNull());
    await act(async () => {
      expect((await old.uploadFiles("", [file(A_TXT)])).cancelled).toBe(true);
      await old.resolveConflicts(new Map([[B_TXT, "replace"]]));
      old.cancelConflicts();
      old.clearUploads();
    });
    expect(result.current.conflicts?.conflicts).toEqual([{ path: B_TXT, is_dir: false }]);
    expect(result.current.uploads[0]?.status).toBe("blocked");
    expect(preflightWorkspaceUpload).toHaveBeenCalledTimes(1);
    expect(uploadWorkspaceFile).not.toHaveBeenCalled();
    await act(async () => {
      await result.current.resolveConflicts(new Map([[B_TXT, "keep_both"]]));
      await caller;
    });
    await expect(caller).resolves.toMatchObject({ cancelled: false });
  });

  it("keeps replacement batches and independent owners live after stale failure", async () => {
    const oldPreflight = deferred<Array<{ path: string; is_dir: boolean }>>();
    preflightWorkspaceUpload.mockReturnValueOnce(oldPreflight.promise);
    const owner = renderHook(({ sessionId }) => useFileUpload(sessionId), {
      initialProps: { sessionId: "sess-1" },
    });
    const sibling = renderHook(() => useFileUpload("sibling"));
    let oldCaller!: ReturnType<typeof owner.result.current.uploadFiles>;
    act(() => {
      oldCaller = owner.result.current.uploadFiles("", [file(A_TXT)]);
    });
    owner.rerender({ sessionId: "sess-2" });
    const newTransport = deferred<{ path: string; size_bytes: number }>();
    uploadWorkspaceFile.mockReturnValueOnce(newTransport.promise);
    let current!: ReturnType<typeof owner.result.current.uploadFiles>;
    act(() => {
      current = owner.result.current.uploadFiles("", [file(B_TXT)]);
    });
    await waitFor(() => expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1));
    await act(async () => {
      oldPreflight.reject(new Error("old failure"));
      await oldCaller;
      await sibling.result.current.uploadFiles("", [file(C_TXT)]);
    });
    expect(owner.result.current.uploads[0]).toMatchObject({
      relativePath: B_TXT,
      status: "uploading",
    });
    expect(sibling.result.current.uploads[0]?.status).toBe("ready");
    await act(async () => {
      newTransport.resolve({ path: B_TXT, size_bytes: 5 });
      await current;
    });
    await expect(oldCaller).resolves.toMatchObject({ cancelled: true });
    await expect(current).resolves.toMatchObject({ cancelled: false });
    expect(owner.result.current.uploads[0]?.status).toBe("ready");
  });
});

describe("useFileUpload live owner controls", () => {
  it("uploads after StrictMode setup cleanup setup", async () => {
    const { result } = renderHook(() => useFileUpload("sess-1"), {
      wrapper: ({ children }) => createElement(StrictMode, null, children),
    });
    await act(async () => {
      const outcome = await result.current.uploadFiles("", [file(A_TXT)]);
      expect(outcome.cancelled).toBe(false);
    });
    expect(uploadWorkspaceFile).toHaveBeenCalledTimes(1);
    expect(result.current.uploads[0]?.status).toBe("ready");
  });

  it("preserves empty and all-skipped outcomes", async () => {
    const { result } = renderHook(() => useFileUpload("sess-1"));
    await act(async () => {
      expect(await result.current.uploadFiles("", [])).toEqual({
        uploaded: [],
        cancelled: false,
        failed: 0,
        skipped: [],
      });
      expect(await result.current.uploadFiles("", [skippedFile()])).toEqual({
        uploaded: [],
        cancelled: false,
        failed: 1,
        skipped: [INVALID_PATH],
      });
    });
    expect(preflightWorkspaceUpload).not.toHaveBeenCalled();
  });
});
