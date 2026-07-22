import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { resolvePRDiffView, usePRDiff, type KeyedPRDiffState } from "./use-pr-diff";

const requestMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  requestMock.mockReset();
});

const staleState: KeyedPRDiffState = {
  sourceKey: "acme/app/1/old",
  files: [
    {
      filename: "src/first-pr.ts",
      status: "modified",
      patch: "@@ -1 +1 @@",
      additions: 1,
      deletions: 1,
    },
  ],
  loading: false,
  error: null,
};

describe("resolvePRDiffView", () => {
  it("masks the previous PR while a newly requested source starts loading", () => {
    expect(resolvePRDiffView(staleState, "acme/app/2/new")).toEqual({
      files: [],
      loading: true,
      error: null,
    });
  });

  it("clears stale files without loading when no PR is requested", () => {
    expect(resolvePRDiffView(staleState, "")).toEqual({
      files: [],
      loading: false,
      error: null,
    });
  });
});

describe("usePRDiff request ownership", () => {
  it("masks A immediately on B and ignores A when its request resolves late", async () => {
    const first = deferred<{ files: KeyedPRDiffState["files"] }>();
    const second = deferred<{ files: KeyedPRDiffState["files"] }>();
    requestMock.mockImplementation((_action: string, payload: { number: number }) =>
      payload.number === 1 ? first.promise : second.promise,
    );
    const { result, rerender } = renderHook(
      ({ number }) => usePRDiff("acme", "app", number, "synced"),
      { initialProps: { number: 1 } },
    );

    rerender({ number: 2 });
    expect(result.current).toMatchObject({ files: [], loading: true, error: null });

    second.resolve({
      files: [
        {
          filename: "src/second-pr.ts",
          status: "modified",
          patch: "@@second@@",
          additions: 1,
          deletions: 0,
        },
      ],
    });
    await waitFor(() => expect(result.current.files[0]?.filename).toBe("src/second-pr.ts"));

    await act(async () => {
      first.resolve({
        files: [
          {
            filename: "src/first-pr.ts",
            status: "modified",
            patch: "@@first@@",
            additions: 1,
            deletions: 0,
          },
        ],
      });
      await first.promise;
    });

    expect(result.current.files[0]?.filename).toBe("src/second-pr.ts");
  });
});
