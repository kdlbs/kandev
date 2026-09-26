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

import { filterGlobalSecrets, useSecrets } from "./use-secrets";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("filterGlobalSecrets", () => {
  it("keeps only global entries for shared profile selectors", () => {
    expect(
      filterGlobalSecrets([
        {
          id: "global",
          name: "Global",
          has_value: true,
          scope: "global",
          created_at: "",
          updated_at: "",
        },
        {
          id: "workspace",
          name: "Workspace",
          has_value: true,
          scope: "workspace",
          created_at: "",
          updated_at: "",
        },
        { id: "legacy", name: "Legacy", has_value: true, created_at: "", updated_at: "" },
      ]),
    ).toMatchObject([{ id: "global" }, { id: "legacy" }]);
  });
});

describe("useSecrets workspace scope", () => {
  beforeEach(() => {
    mockListSecrets.mockReset();
  });

  it("clears the previous workspace while the next request is loading", async () => {
    const first = deferred<SecretListItem[]>();
    const second = deferred<SecretListItem[]>();
    mockListSecrets.mockImplementation(({ workspaceId }: { workspaceId?: string }) =>
      workspaceId === "workspace-a" ? first.promise : second.promise,
    );

    const view = renderHook(({ workspaceId }) => useSecrets("workspace", workspaceId), {
      initialProps: { workspaceId: "workspace-a" },
    });
    await act(async () => {
      first.resolve([
        {
          id: "secret-a",
          name: "A",
          has_value: true,
          scope: "workspace",
          created_at: "",
          updated_at: "",
        },
      ]);
      await first.promise;
    });
    await waitFor(() => expect(view.result.current.items).toHaveLength(1));

    const firstRequest = mockListSecrets.mock.calls[0][0] as { init?: { signal?: AbortSignal } };
    view.rerender({ workspaceId: "workspace-b" });
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.loaded).toBe(false);
    expect(firstRequest.init?.signal?.aborted).toBe(true);

    await act(async () => {
      second.resolve([
        {
          id: "secret-b",
          name: "B",
          has_value: true,
          scope: "workspace",
          created_at: "",
          updated_at: "",
        },
      ]);
      await second.promise;
    });
    await waitFor(() => expect(view.result.current.items[0]?.id).toBe("secret-b"));
  });
});
