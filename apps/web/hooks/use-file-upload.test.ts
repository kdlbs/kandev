import { act, renderHook, waitFor } from "@testing-library/react";
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
