import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SecretListItem } from "@/lib/types/http-secrets";

const { mockListSecrets, storeState } = vi.hoisted(() => ({
  mockListSecrets: vi.fn(),
  storeState: {
    secrets: { items: [] as SecretListItem[], loaded: true, loading: false },
    setSecrets: vi.fn(),
    setSecretsLoading: vi.fn(),
    addSecret: vi.fn(),
    updateSecret: vi.fn(),
    removeSecret: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/secrets-api", () => ({ listSecrets: mockListSecrets }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { secretNameConflict, useSecretDestinationNames } from "./use-secret-destination-names";

/** Deferred promise helper for controlling async test resolution. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("secretNameConflict", () => {
  it("matches exact trimmed names only", () => {
    expect(secretNameConflict(["API Key"], "API Key")).toBe(true);
    expect(secretNameConflict(["API Key"], "  API Key  ")).toBe(true);
    expect(secretNameConflict(["API Key"], "api key")).toBe(false);
    expect(secretNameConflict([], "API Key")).toBe(false);
    expect(secretNameConflict(["API Key"], "   ")).toBe(false);
  });
});

describe("useSecretDestinationNames", () => {
  beforeEach(() => {
    mockListSecrets.mockReset();
    storeState.secrets.items = [];
  });

  it("reads Global names from the store and excludes workspace entries", () => {
    storeState.secrets.items = [
      { id: "g1", name: "Global Key" } as SecretListItem,
      { id: "w1", name: "Workspace Key", scope: "workspace" } as SecretListItem,
    ];
    const view = renderHook(() => useSecretDestinationNames("global"));
    expect(view.result.current.names).toEqual(["Global Key"]);
    expect(view.result.current.loaded).toBe(true);
    expect(view.result.current.conflict("Global Key")).toBe(true);
    expect(view.result.current.conflict("Workspace Key")).toBe(false);
    expect(mockListSecrets).not.toHaveBeenCalled();
  });

  it("fetches workspace names on demand and caches per workspace", async () => {
    const pending = deferred<SecretListItem[]>();
    mockListSecrets.mockReturnValue(pending.promise);
    const view = renderHook(
      ({ workspaceId }: { workspaceId?: string }) =>
        useSecretDestinationNames("workspace", workspaceId),
      { initialProps: { workspaceId: "workspace-a" } },
    );
    expect(view.result.current.loaded).toBe(false);
    expect(view.result.current.conflict("anything")).toBe(false);

    await act(async () => pending.resolve([{ id: "w1", name: "Taken" } as SecretListItem]));
    await waitFor(() => expect(view.result.current.loaded).toBe(true));
    expect(view.result.current.conflict("Taken")).toBe(true);
    expect(view.result.current.conflict("other")).toBe(false);

    // Same workspace does not refetch.
    view.rerender({ workspaceId: "workspace-a" });
    expect(mockListSecrets).toHaveBeenCalledTimes(1);

    // A different workspace refetches.
    view.rerender({ workspaceId: "workspace-b" });
    expect(mockListSecrets).toHaveBeenCalledTimes(2);
  });

  it("refetches the same workspace when the refresh key changes", async () => {
    mockListSecrets.mockResolvedValue([]);
    const view = renderHook(
      ({ refreshKey }: { refreshKey: number }) =>
        useSecretDestinationNames("workspace", "workspace-a", refreshKey),
      { initialProps: { refreshKey: 0 } },
    );
    await waitFor(() => expect(view.result.current.loaded).toBe(true));
    expect(mockListSecrets).toHaveBeenCalledTimes(1);

    // Same key: cached.
    view.rerender({ refreshKey: 0 });
    expect(mockListSecrets).toHaveBeenCalledTimes(1);

    // New transfer session: the per-workspace cache must be invalidated so a
    // secret copied in the previous session shows up as a conflict now.
    view.rerender({ refreshKey: 1 });
    await waitFor(() => expect(mockListSecrets).toHaveBeenCalledTimes(2));
  });
});
