import { createElement, type ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { StateProvider } from "@/components/state-provider";
import type { AgentRuntimeAvailability } from "@/lib/types/agent-runtime";

const mocks = vi.hoisted(() => ({ retryAgentRuntime: vi.fn() }));

vi.mock("@/lib/api/domains/system-api", () => ({ retryAgentRuntime: mocks.retryAgentRuntime }));

import { useAgentRuntimeRecovery } from "./use-agent-runtime-recovery";

const unavailable: AgentRuntimeAvailability = {
  status: "unavailable",
  reason: "recovery_exhausted",
  boot_id: "boot-1",
  runtime_epoch: 4,
  revision: 9,
  retry_allowed: true,
};

const recovering: AgentRuntimeAvailability = {
  status: "recovering",
  boot_id: "boot-1",
  runtime_epoch: 5,
  revision: 10,
  recovery_id: "recovery-1",
};

function wrapper(role: string) {
  const initialState = {
    agentRuntime: unavailable,
    auth: {
      mode: "enabled" as const,
      authenticated: true,
      user: {
        id: "user-1",
        email: "user@example.test",
        display_name: "Test user",
        role,
        status: "active",
      },
    },
  };
  return function RecoveryTestProvider({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState, children });
  };
}

describe("useAgentRuntimeRecovery", () => {
  beforeEach(() => {
    mocks.retryAgentRuntime.mockReset();
  });

  it("submits the observed revision and applies the accepted snapshot", async () => {
    mocks.retryAgentRuntime.mockResolvedValue(recovering);
    const { result } = renderHook(() => useAgentRuntimeRecovery(), { wrapper: wrapper("admin") });

    expect(result.current.canRetry).toBe(true);
    await act(async () => result.current.retry());

    expect(mocks.retryAgentRuntime).toHaveBeenCalledWith({
      boot_id: "boot-1",
      runtime_epoch: 4,
      revision: 9,
      request_id: expect.stringMatching(/^[0-9a-f-]{36}$/),
    });
    expect(result.current.isRecovering).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("shows a stale status response without retrying it automatically", async () => {
    mocks.retryAgentRuntime.mockRejectedValue(
      new ApiError("stale", 409, { error_code: "stale_runtime_snapshot" }),
    );
    const { result } = renderHook(() => useAgentRuntimeRecovery(), { wrapper: wrapper("admin") });

    await act(async () => result.current.retry());

    expect(result.current.error).toBe("stale");
    expect(mocks.retryAgentRuntime).toHaveBeenCalledTimes(1);
  });

  it("does not expose or invoke recovery for a non-admin user", async () => {
    const { result } = renderHook(() => useAgentRuntimeRecovery(), { wrapper: wrapper("member") });

    expect(result.current.isAdmin).toBe(false);
    expect(result.current.canRetry).toBe(false);
    await act(async () => result.current.retry());

    expect(mocks.retryAgentRuntime).not.toHaveBeenCalled();
  });
});
