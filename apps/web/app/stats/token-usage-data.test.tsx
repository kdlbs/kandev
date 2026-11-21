import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TokenUsageResponse } from "@/lib/types/http";
import { deferred, retainedReport, retainedRow } from "./token-usage.test-helpers";

const mocks = vi.hoisted(() => ({ fetchTokenUsage: vi.fn() }));
vi.mock("@/lib/api/domains/stats-api", () => mocks);
import { useTokenUsage } from "./token-usage-data";

beforeEach(() => mocks.fetchTokenUsage.mockReset());

describe("retained usage publication", () => {
  it("keeps the complete saved generation during retry, preparation and a failed refresh", async () => {
    const saved = retainedReport();
    const rebuilding = deferred<TokenUsageResponse>();
    mocks.fetchTokenUsage.mockResolvedValueOnce(saved).mockReturnValueOnce(rebuilding.promise);
    const { result, rerender } = renderHook(
      ({ retry }) => useTokenUsage("workspace-a", "month", retry),
      { initialProps: { retry: 0 } },
    );
    await waitFor(() => expect(result.current.status).toBe("ready"));
    rerender({ retry: 1 });
    expect(result.current).toMatchObject({ status: "ready", data: saved, refreshing: true });
    await act(async () =>
      rebuilding.resolve(
        retainedReport({
          projection_status: "preparing",
          projection_generation: "",
          projection_pending: true,
          rows: [],
          has_history: false,
        }),
      ),
    );
    expect(result.current).toMatchObject({
      status: "ready",
      data: {
        rows: saved.rows,
        projection_generation: saved.projection_generation,
        projection_pending: true,
      },
    });
    mocks.fetchTokenUsage.mockRejectedValueOnce(new Error("offline"));
    rerender({ retry: 2 });
    await waitFor(() =>
      expect(result.current).toMatchObject({
        status: "ready",
        error: "offline",
        data: { rows: saved.rows },
      }),
    );
  });

  it("discards saved values on access denial and never carries them to another workspace", async () => {
    mocks.fetchTokenUsage.mockResolvedValueOnce(retainedReport());
    const { result, rerender } = renderHook(
      ({ workspace, retry }) => useTokenUsage(workspace, "month", retry),
      { initialProps: { workspace: "a", retry: 0 } },
    );
    await waitFor(() => expect(result.current.status).toBe("ready"));
    mocks.fetchTokenUsage.mockRejectedValueOnce(
      Object.assign(new Error("denied"), { status: 403 }),
    );
    rerender({ workspace: "a", retry: 1 });
    await waitFor(() => expect(result.current).toMatchObject({ status: "error" }));
    const next = deferred<TokenUsageResponse>();
    mocks.fetchTokenUsage.mockReturnValueOnce(next.promise);
    rerender({ workspace: "b", retry: 1 });
    expect(result.current).toMatchObject({ status: "loading" });
    expect(result.current).not.toHaveProperty("data");
    await act(async () => next.resolve(retainedReport({ rows: [] })));
  });

  it("shows a failed first-build status read as an error when no complete values exist", async () => {
    mocks.fetchTokenUsage.mockResolvedValueOnce(
      retainedReport({
        projection_status: "preparing",
        projection_generation: "",
        projection_pending: true,
        rows: [],
      }),
    );
    const { result, rerender } = renderHook(({ retry }) => useTokenUsage("a", "month", retry), {
      initialProps: { retry: 0 },
    });
    await waitFor(() => expect(result.current.status).toBe("ready"));
    mocks.fetchTokenUsage.mockRejectedValueOnce(new Error("offline"));
    rerender({ retry: 1 });
    await waitFor(() =>
      expect(result.current).toMatchObject({ status: "error", message: "offline" }),
    );
  });
});

describe("retained usage response ownership", () => {
  it("accepts a later complete opaque generation and ignores a late response from the old workspace", async () => {
    const old = deferred<TokenUsageResponse>();
    mocks.fetchTokenUsage
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce(retainedReport({ projection_generation: "z" }));
    const { result, rerender } = renderHook(
      ({ workspace, retry }) => useTokenUsage(workspace, "month", retry),
      { initialProps: { workspace: "old", retry: 0 } },
    );
    rerender({ workspace: "current", retry: 0 });
    await waitFor(() =>
      expect(result.current).toMatchObject({
        status: "ready",
        data: { projection_generation: "z" },
      }),
    );
    await act(async () => old.resolve(retainedReport({ projection_generation: "foreign" })));
    expect(result.current).toMatchObject({ status: "ready", data: { projection_generation: "z" } });
    mocks.fetchTokenUsage.mockResolvedValueOnce(
      retainedReport({ projection_generation: "a", totals: retainedRow({ total_tokens: 7 }) }),
    );
    rerender({ workspace: "current", retry: 1 });
    await waitFor(() =>
      expect(result.current).toMatchObject({
        status: "ready",
        data: { projection_generation: "a", totals: { total_tokens: 7 } },
      }),
    );
  });
});

describe("complete usage trend pagination", () => {
  it("rejects a terminal response that omits promised filtered rows", async () => {
    mocks.fetchTokenUsage.mockResolvedValueOnce(retainedReport({ total_rows: 3, has_more: false }));
    const { result } = renderHook(() => useTokenUsage("a", "all", 0, "day", { allPages: true }));
    await waitFor(() => expect(result.current.status).toBe("error"));
  });
  it("does not combine pages from different opaque complete generations", async () => {
    mocks.fetchTokenUsage
      .mockResolvedValueOnce(retainedReport({ has_more: true, total_rows: 2 }))
      .mockResolvedValueOnce(
        retainedReport({
          projection_generation: "generation-a/opaque",
          rows: [retainedRow({ key: "next" })],
        }),
      );
    const { result } = renderHook(() => useTokenUsage("a", "all", 0, "day", { allPages: true }));
    await waitFor(() => expect(result.current.status).toBe("error"));
  });

  it("fails an incomplete trend page instead of declaring truncated rows complete", async () => {
    mocks.fetchTokenUsage
      .mockResolvedValueOnce(retainedReport({ has_more: true, total_rows: 2 }))
      .mockResolvedValueOnce(retainedReport({ has_more: true, rows: [] }));
    const { result } = renderHook(() => useTokenUsage("a", "all", 0, "day", { allPages: true }));
    await waitFor(() => expect(result.current.status).toBe("error"));
  });

  it("loads every trend page and keeps server filtered totals", async () => {
    const first = retainedReport({ has_more: true, total_rows: 2 });
    const next = retainedRow({ key: "next", period: "2026-10-13" });
    mocks.fetchTokenUsage
      .mockResolvedValueOnce(first)
      .mockResolvedValueOnce(retainedReport({ rows: [next] }));
    const { result } = renderHook(() => useTokenUsage("a", "all", 0, "day", { allPages: true }));
    await waitFor(() =>
      expect(result.current).toMatchObject({
        status: "ready",
        data: { rows: [...first.rows, next], totals: first.totals, has_more: false },
      }),
    );
  });
});

it("pins subsequent trend pages to the server publication and resolved time bounds", async () => {
  const first = retainedReport({
    has_more: true,
    total_rows: 2,
    range_start: "2026-10-01T00:00:00Z",
    range_end: "2026-10-29T04:00:00.123456789Z",
  });
  mocks.fetchTokenUsage
    .mockResolvedValueOnce(first)
    .mockResolvedValueOnce(retainedReport({ rows: [retainedRow({ key: "next" })] }));
  const { result } = renderHook(() => useTokenUsage("a", "month", 0, "day", { allPages: true }));
  await waitFor(() => expect(result.current.status).toBe("ready"));
  expect(mocks.fetchTokenUsage.mock.calls[1][4]).toMatchObject({
    projectionGeneration: first.projection_generation,
    start: first.range_start,
    end: first.range_end,
  });
});
