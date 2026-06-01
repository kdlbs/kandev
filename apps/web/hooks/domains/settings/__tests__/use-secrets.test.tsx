import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSecrets, useCreateSecret, useDeleteSecret } from "../use-secrets";
import { qk } from "@/lib/query/keys";
import type { SecretListItem } from "@/lib/types/http-secrets";

const MOCK_SECRETS: SecretListItem[] = [
  {
    id: "sec-1",
    name: "MY_SECRET",
    has_value: true,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "",
  },
];

vi.mock("@/lib/api/domains/secrets-api", () => ({
  listSecrets: vi.fn().mockResolvedValue([] as SecretListItem[]),
  createSecret: vi.fn(),
  updateSecret: vi.fn(),
  deleteSecret: vi.fn(),
  revealSecret: vi.fn(),
}));

import * as secretsApi from "@/lib/api/domains/secrets-api";

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

function makeClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: Infinity },
      mutations: { retry: false },
    },
  });
}

describe("useSecrets", () => {
  it("returns pre-seeded cache data immediately", () => {
    const qc = makeClient();
    // Seed using the exact same key factory the hook uses
    qc.setQueryData(qk.settings.secrets(), MOCK_SECRETS);

    const { result } = renderHook(() => useSecrets(), { wrapper: makeWrapper(qc) });

    expect(result.current.items).toEqual(MOCK_SECRETS);
    expect(result.current.loaded).toBe(true);
  });

  it("returns empty array and loaded=false before fetch completes", () => {
    vi.mocked(secretsApi.listSecrets).mockReturnValue(new Promise(() => {}) as never);
    const qc = makeClient();
    const { result } = renderHook(() => useSecrets(), { wrapper: makeWrapper(qc) });
    expect(result.current.items).toEqual([]);
    expect(result.current.loaded).toBe(false);
  });
});

describe("useCreateSecret + useSecrets combined", () => {
  const NEW_SECRET: SecretListItem = {
    id: "sec-2",
    name: "NEW_SECRET",
    has_value: true,
    created_at: "",
    updated_at: "",
  };

  beforeEach(() => {
    vi.mocked(secretsApi.createSecret).mockResolvedValue(NEW_SECRET);
  });

  it("adds the returned secret and the list hook reflects it", async () => {
    const qc = makeClient();
    qc.setQueryData(qk.settings.secrets(), MOCK_SECRETS);

    const { result: listResult } = renderHook(() => useSecrets(), { wrapper: makeWrapper(qc) });
    const { result: mutResult } = renderHook(() => useCreateSecret(), {
      wrapper: makeWrapper(qc),
    });

    act(() => {
      mutResult.current.mutate({ name: "NEW_SECRET", value: "val" });
    });

    await waitFor(() => expect(mutResult.current.isSuccess).toBe(true));
    // After mutation, list hook should reflect the new item
    await waitFor(() => {
      expect(listResult.current.items.map((s) => s.id)).toContain("sec-2");
    });
  });
});

describe("useDeleteSecret + useSecrets combined", () => {
  beforeEach(() => {
    vi.mocked(secretsApi.deleteSecret).mockResolvedValue(undefined);
  });

  it("removes the secret and the list hook reflects the removal", async () => {
    const qc = makeClient();
    qc.setQueryData(qk.settings.secrets(), MOCK_SECRETS);

    const { result: listResult } = renderHook(() => useSecrets(), { wrapper: makeWrapper(qc) });
    const { result: mutResult } = renderHook(() => useDeleteSecret(), {
      wrapper: makeWrapper(qc),
    });

    act(() => {
      mutResult.current.mutate("sec-1");
    });

    await waitFor(() => expect(mutResult.current.isSuccess).toBe(true));
    await waitFor(() => {
      expect(listResult.current.items.map((s) => s.id)).not.toContain("sec-1");
    });
  });
});
