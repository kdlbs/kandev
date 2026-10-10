import { createElement, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { SentryConfig } from "@/lib/types/sentry";

const mocks = vi.hoisted(() => ({ listSentryInstances: vi.fn() }));
vi.mock("@/lib/api/domains/sentry-api", () => ({
  listSentryInstances: mocks.listSentryInstances,
}));

import { useSentryInstances } from "./use-sentry-availability";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function sentryInstance(): SentryConfig {
  return {
    id: "sentry-instance-1",
    workspaceId: "ws-1",
    name: "Production",
    authMethod: "auth_token",
    url: "https://sentry.io",
    hasSecret: true,
    lastOk: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

beforeEach(() => {
  window.localStorage.clear();
  mocks.listSentryInstances.mockReset();
});

afterEach(() => window.localStorage.clear());

describe("useSentryInstances", () => {
  it("shares one workspace read between task consumers", async () => {
    let resolveInstances!: (instances: SentryConfig[]) => void;
    mocks.listSentryInstances.mockReturnValue(
      new Promise((resolve) => {
        resolveInstances = resolve;
      }),
    );

    const { result } = renderHook(
      () => [useSentryInstances("ws-1"), useSentryInstances("ws-1")] as const,
      { wrapper },
    );

    await waitFor(() => expect(mocks.listSentryInstances).toHaveBeenCalledTimes(1));
    await act(async () => resolveInstances([sentryInstance()]));
    await waitFor(() => expect(result.current[0].available).toBe(true));
    expect(result.current[1].healthy.map((instance) => instance.id)).toEqual(["sentry-instance-1"]);
  });

  it("does not probe a disabled workspace", async () => {
    window.localStorage.setItem("kandev:sentry:enabled:v1:ws-1", "false");
    const { result } = renderHook(() => useSentryInstances("ws-1"), { wrapper });

    await waitFor(() => expect(result.current.available).toBe(false));
    expect(mocks.listSentryInstances).not.toHaveBeenCalled();
  });
});
