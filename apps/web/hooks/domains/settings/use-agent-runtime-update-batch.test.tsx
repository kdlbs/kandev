import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useAgentRuntimeUpdateBatch } from "./use-agent-runtime-update-batch";
import { runtimeBatchAuthority } from "@/lib/agents/runtime-update-batch";

const { listJobs } = vi.hoisted(() => ({ listJobs: vi.fn() }));
const AGENT = "claude-acp";
const RUNTIME = "npm:@example/claude";
const PACKAGE = "@example/claude";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    updateAgent: vi.fn(),
    listAgentUpdateJobs: (...args: unknown[]) => listJobs(...args),
  };
});

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function Capture({ onStore }: { onStore: (store: ReturnType<typeof useAppStoreApi>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

beforeEach(() => {
  listJobs.mockReset();
  listJobs.mockResolvedValue({ jobs: [] });
});

describe("useAgentRuntimeUpdateBatch", () => {
  it("publishes review state through the store-owned coordinator", async () => {
    const { result } = renderHook(() => useAgentRuntimeUpdateBatch(), { wrapper });
    await waitFor(() => expect(listJobs).toHaveBeenCalled());
    act(() => {
      result.current.beginReview([
        {
          key: `${AGENT}:${RUNTIME}`,
          agentName: AGENT,
          displayName: "Claude",
          runtimeId: RUNTIME,
          package: PACKAGE,
          currentVersion: "1.0.0",
          effectiveVersion: "1.0.0",
          targetVersion: "1.1.0",
          reviewedRuntime: {
            runtime_id: RUNTIME,
            package: PACKAGE,
            current_version: "1.0.0",
            effective_version: "1.0.0",
            active_version: "1.0.0",
          },
        },
      ]);
    });
    await waitFor(() => expect(result.current.batch.status).toBe("review"));
    expect(Object.keys(result.current.batch.rows)).toEqual([`${AGENT}:${RUNTIME}`]);
  });
});

describe("useAgentRuntimeUpdateBatch reconnect recovery", () => {
  it("recovers a missed finish event on WebSocket reconnect without window online", async () => {
    let store: ReturnType<typeof useAppStoreApi> | undefined;
    const completed = {
      job_id: "job-claude",
      agent_name: AGENT,
      status: "succeeded" as const,
      operation: "update" as const,
      target_version: "1.1.0",
      started_at: "2026-01-01T00:00:00Z",
      finished_at: "2026-01-01T00:01:00Z",
    };
    listJobs.mockResolvedValueOnce({ jobs: [] }).mockResolvedValueOnce({ jobs: [completed] });
    const { result } = renderHook(() => useAgentRuntimeUpdateBatch(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <StateProvider>
          <Capture onStore={(next) => (store = next)} />
          {children}
        </StateProvider>
      ),
    });
    await waitFor(() => expect(store).toBeDefined());
    await waitFor(() => expect(listJobs).toHaveBeenCalledOnce());
    const target = {
      key: `${AGENT}:${RUNTIME}`,
      agentName: AGENT,
      displayName: "Claude",
      runtimeId: RUNTIME,
      package: PACKAGE,
      currentVersion: "1.0.0",
      effectiveVersion: "1.0.0",
      targetVersion: "1.1.0",
      reviewedRuntime: {
        runtime_id: RUNTIME,
        package: PACKAGE,
        current_version: "1.0.0",
        effective_version: "1.0.0",
        active_version: "1.0.0",
      },
    };
    act(() => {
      const state = store!.getState();
      state.setRuntimeUpdateBatch({
        status: "running",
        authority: runtimeBatchAuthority(state),
        order: [target.key],
        rows: { [target.key]: { ...target, phase: "queued", jobId: "job-claude" } },
      });
      state.setConnectionStatus("reconnecting");
    });
    act(() => {
      store!.getState().setConnectionStatus("connected");
    });
    await waitFor(() => expect(listJobs).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.batch.status).toBe("complete"));
    expect(result.current.batch.rows[target.key]?.phase).toBe("succeeded");
  });
});
