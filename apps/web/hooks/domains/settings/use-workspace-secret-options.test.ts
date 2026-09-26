import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SecretListItem } from "@/lib/types/http-secrets";

const { mockListSecrets } = vi.hoisted(() => ({ mockListSecrets: vi.fn() }));

vi.mock("@/lib/api/domains/secrets-api", () => ({ listSecrets: mockListSecrets }));

import { useWorkspaceSecretOptions } from "./use-workspace-secret-options";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("useWorkspaceSecretOptions", () => {
  beforeEach(() => mockListSecrets.mockReset());

  it("clears and cancels a workspace request when the workspace is removed", () => {
    const pending = deferred<SecretListItem[]>();
    mockListSecrets.mockReturnValue(pending.promise);
    const initialProps: { workspaceId?: string } = { workspaceId: "workspace-a" };
    const view = renderHook(
      ({ workspaceId }: { workspaceId?: string }) => useWorkspaceSecretOptions(workspaceId),
      {
        initialProps,
      },
    );
    const request = mockListSecrets.mock.calls[0][0] as { init?: { signal?: AbortSignal } };

    view.rerender({ workspaceId: undefined });
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.loaded).toBe(true);
    expect(view.result.current.loading).toBe(false);
    expect(request.init?.signal?.aborted).toBe(true);

    act(() => pending.resolve([]));
  });
});
