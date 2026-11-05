import { createElement, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSandboxExecutorReads } from "./use-sandbox-executor-reads";

const api = vi.hoisted(() => ({
  fetchSandboxPolicies: vi.fn(),
  fetchSandboxProviders: vi.fn(),
  fetchSandboxRevision: vi.fn(),
}));
vi.mock("@/lib/api/domains/sandbox-api", () => api);

function wrapper({ children }: { children: ReactNode }) {
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  return createElement(QueryClientProvider, { client }, children);
}

describe("executor policy workspace browsing", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.fetchSandboxProviders.mockResolvedValue([]);
    api.fetchSandboxPolicies.mockResolvedValue([]);
  });
  it("loads the selected workspace and isolates a later selection", async () => {
    let completeFirst!: (value: unknown[]) => void;
    api.fetchSandboxPolicies.mockImplementation((workspace: string) =>
      workspace === "first"
        ? new Promise((resolve) => {
            completeFirst = resolve;
          })
        : Promise.resolve([]),
    );
    const { result, rerender } = renderHook(
      ({ workspace }) => useSandboxExecutorReads("operator", undefined, true, workspace),
      { initialProps: { workspace: "first" }, wrapper },
    );
    await waitFor(() =>
      expect(api.fetchSandboxPolicies).toHaveBeenCalledWith("first", expect.anything()),
    );
    rerender({ workspace: "second" });
    await waitFor(() =>
      expect(api.fetchSandboxPolicies).toHaveBeenCalledWith("second", expect.anything()),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      completeFirst([{ id: "first-policy" }]);
    });
    await waitFor(() => expect(result.current.revisions).toEqual([]));
  });
  it("does not browse private policies for a read-only operator", async () => {
    const { result } = renderHook(
      () => useSandboxExecutorReads("member", undefined, false, "workspace"),
      { wrapper },
    );
    expect(result.current.loading).toBe(false);
    expect(api.fetchSandboxPolicies).not.toHaveBeenCalled();
  });
});
