import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  TurnChangeContent,
  TurnChangeFilesPage,
  TurnChangeSetSummary,
} from "@/lib/types/turn-changes";
import { listTurnChangeFiles, readTurnChangeContent } from "@/lib/api/domains/turn-changes-api";
import { useTurnChangeContent, useTurnChangeFiles } from "./use-turn-change-files";

vi.mock("@/lib/api/domains/turn-changes-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/turn-changes-api")>()),
  listTurnChangeFiles: vi.fn(),
  readTurnChangeContent: vi.fn(),
}));
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
function summary(id = "first"): TurnChangeSetSummary {
  return {
    id,
    task_id: "task",
    session_id: "session",
    turn_id: id,
    revision: 1,
    availability: "ready",
    complete: true,
    summary_complete: true,
    content_complete: true,
    turn_ordinal: 1,
    fallback_anchor: id,
    file_count: 2,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 1,
    repositories: [
      {
        id: "repo",
        checkout_id: "checkout",
        availability: "ready",
        enumeration_complete: true,
        comparison_complete: true,
        content_complete: true,
      },
    ],
  };
}
function page(id: string, nextOffset?: number): TurnChangeFilesPage {
  return {
    files: [
      {
        id,
        repository_change_id: "repo",
        checkout_id: "checkout",
        path: `${id}.ts`,
        kind: "modified",
        content_availability: "ready",
      },
    ],
    total: 2,
    offset: 0,
    limit: 100,
    next_offset: nextOffset,
  };
}
beforeEach(() => {
  vi.mocked(listTurnChangeFiles).mockReset();
});
afterEach(cleanup);

describe("turn file request ownership", () => {
  it("ignores an initial error from a previous turn", async () => {
    const old = deferred<TurnChangeFilesPage>();
    vi.mocked(listTurnChangeFiles)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce(page("new"));
    const { result, rerender } = renderHook(({ value }) => useTurnChangeFiles("session", value), {
      initialProps: { value: summary() },
    });
    await act(async () => rerender({ value: summary("second") }));
    await act(async () => old.reject(new Error("old request")));
    expect(result.current.error).toBeNull();
    expect(result.current.filesByRepository.repo).toEqual(page("new").files);
  });
  it.each(["resolve", "reject"] as const)(
    "ignores stale pagination %s while a new turn loads",
    async (outcome) => {
      const oldPage = deferred<TurnChangeFilesPage>();
      const newPage = deferred<TurnChangeFilesPage>();
      vi.mocked(listTurnChangeFiles)
        .mockResolvedValueOnce(page("old", 1))
        .mockReturnValueOnce(oldPage.promise)
        .mockReturnValueOnce(newPage.promise);
      const { result, rerender } = renderHook(({ value }) => useTurnChangeFiles("session", value), {
        initialProps: { value: summary() },
      });
      await act(async () => {});
      let pending!: Promise<void>;
      act(() => {
        pending = result.current.loadMore();
      });
      rerender({ value: summary("second") });
      await act(async () => {
        if (outcome === "resolve") oldPage.resolve(page("stale"));
        else oldPage.reject(new Error("old pagination"));
        await pending;
      });
      expect(result.current.loading).toBe(true);
      expect(result.current.error).toBeNull();
      expect(result.current.filesByRepository).toEqual({});
      await act(async () => newPage.resolve(page("new")));
      expect(result.current.filesByRepository.repo).toEqual(page("new").files);
    },
  );
  it("settles loading when repositories become empty", async () => {
    const pending = deferred<TurnChangeFilesPage>();
    vi.mocked(listTurnChangeFiles).mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(({ value }) => useTurnChangeFiles("session", value), {
      initialProps: { value: summary() },
    });
    rerender({ value: { ...summary("empty"), repositories: [] } });
    expect(result.current.loading).toBe(false);
    await act(async () => pending.resolve(page("old")));
    expect(result.current.filesByRepository).toEqual({});
  });
  it("does not append a page twice for same-tick load-more calls", async () => {
    const next = deferred<TurnChangeFilesPage>();
    vi.mocked(listTurnChangeFiles)
      .mockResolvedValueOnce(page("first", 1))
      .mockReturnValue(next.promise);
    const value = summary();
    const { result } = renderHook(() => useTurnChangeFiles("session", value));
    await act(async () => {});
    let requests!: Promise<void>[];
    act(() => {
      requests = [result.current.loadMore(), result.current.loadMore()];
    });
    await act(async () => {
      next.resolve(page("next"));
      await Promise.all(requests);
    });
    expect(result.current.filesByRepository.repo.map((file) => file.id)).toEqual(["first", "next"]);
  });
});

describe("turn file retries", () => {
  it("retries a failed initial file request", async () => {
    vi.mocked(listTurnChangeFiles)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(page("retried"));
    const value = summary();
    const { result } = renderHook(() => useTurnChangeFiles("session", value));
    await act(async () => {});
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.retry).toBeTypeOf("function");
    await act(async () => result.current.retry());
    expect(result.current.filesByRepository.repo).toEqual(page("retried").files);
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });
  it("retains loaded files and cursor after pagination failure for retry", async () => {
    vi.mocked(listTurnChangeFiles)
      .mockResolvedValueOnce(page("first", 1))
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(page("next"));
    const value = summary();
    const { result } = renderHook(() => useTurnChangeFiles("session", value));
    await act(async () => {});
    await act(async () => result.current.loadMore());
    expect(result.current.filesByRepository.repo).toEqual(page("first").files);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.retry).toBeTypeOf("function");
    await act(async () => result.current.retry());
    expect(result.current.filesByRepository.repo.map((file) => file.id)).toEqual(["first", "next"]);
    expect(result.current.error).toBeNull();
  });
});

describe("partial repository file failures", () => {
  it("keeps healthy repository files and recovers the failed sibling on retry", async () => {
    const value = summary();
    value.repositories.push({ ...value.repositories[0], id: "other" });
    vi.mocked(listTurnChangeFiles)
      .mockResolvedValueOnce(page("healthy", 1))
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(page("healthy", 1))
      .mockResolvedValueOnce(page("recovered"));
    const { result } = renderHook(() => useTurnChangeFiles("session", value));
    await act(async () => {});
    expect(result.current.filesByRepository.repo).toEqual(page("healthy").files);
    expect(result.current.fileTotalsByRepository.repo).toBe(2);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.loading).toBe(false);
    expect(result.current.retry).toBeTypeOf("function");
    await act(async () => result.current.retry());
    expect(result.current.filesByRepository.other).toEqual(page("recovered").files);
    expect(result.current.error).toBeNull();
  });
});

describe("turn content loading", () => {
  it("settles loading after deselecting a file and ignores its late content", async () => {
    const pending = deferred<TurnChangeContent>();
    vi.mocked(readTurnChangeContent).mockReturnValueOnce(pending.promise);
    const target = { sessionId: "session", changeSetId: "changes" };
    const { result, rerender } = renderHook(
      ({ id }: { id: string | null }) => useTurnChangeContent(target, id, false),
      { initialProps: { id: "file" as string | null } },
    );
    expect(result.current.loading).toBe(true);
    rerender({ id: null });
    expect(result.current.loading).toBe(false);
    await act(async () =>
      pending.resolve({
        file_change_id: "file",
        variant: "canonical_patch",
        content: btoa("old patch"),
        digest: "digest",
      }),
    );
    expect(result.current.patch).toBeNull();
    expect(result.current.error).toBeNull();
  });
});
