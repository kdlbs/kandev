import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { AutomationRun } from "@/lib/types/automation";

// ---------------------------------------------------------------------------
// Mock the automation API
// ---------------------------------------------------------------------------

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: vi.fn(),
  createAutomation: vi.fn(),
  updateAutomation: vi.fn(),
  deleteAutomation: vi.fn(),
  enableAutomation: vi.fn(),
  disableAutomation: vi.fn(),
  triggerAutomation: vi.fn(),
  listAutomationRuns: vi.fn(),
}));

import { listAutomationRuns } from "@/lib/api/domains/automation-api";
const mockListAutomationRuns = vi.mocked(listAutomationRuns);

import { useAutomationRuns } from "./use-automation-runs";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeRun(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    automation_id: "auto-1",
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
}

function makeWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.resetAllMocks();
});

describe("useAutomationRuns", () => {
  it("returns empty runs when automationId is null", () => {
    const client = makeClient();

    const { result } = renderHook(() => useAutomationRuns(null), {
      wrapper: makeWrapper(client),
    });

    expect(result.current.runs).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(mockListAutomationRuns).not.toHaveBeenCalled();
  });

  it("fetches runs when automationId is provided", async () => {
    const run = makeRun();
    mockListAutomationRuns.mockResolvedValue([run]);
    const client = makeClient();

    const { result } = renderHook(() => useAutomationRuns("auto-1"), {
      wrapper: makeWrapper(client),
    });

    // Initially loading
    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.runs).toEqual([run]);
    expect(mockListAutomationRuns).toHaveBeenCalledWith("auto-1");
  });

  it("returns empty runs on fetch error", async () => {
    mockListAutomationRuns.mockRejectedValue(new Error("network error"));
    const client = makeClient();

    const { result } = renderHook(() => useAutomationRuns("auto-1"), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.runs).toEqual([]);
  });

  it("refresh() re-fetches runs", async () => {
    const run1 = makeRun({ id: "run-1" });
    const run2 = makeRun({ id: "run-2" });
    mockListAutomationRuns.mockResolvedValue([run1]);

    const client = makeClient();

    const { result } = renderHook(() => useAutomationRuns("auto-1"), {
      wrapper: makeWrapper(client),
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.runs).toHaveLength(1);

    // Second call returns more runs
    mockListAutomationRuns.mockResolvedValue([run1, run2]);

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => {
      expect(result.current.runs).toHaveLength(2);
    });

    expect(mockListAutomationRuns).toHaveBeenCalledTimes(2);
  });

  it("uses separate cache entries per automationId", async () => {
    const run1 = makeRun({ id: "run-1", automation_id: "auto-1" });
    const run2 = makeRun({ id: "run-2", automation_id: "auto-2" });

    mockListAutomationRuns.mockImplementation((id: string) =>
      id === "auto-1" ? Promise.resolve([run1]) : Promise.resolve([run2]),
    );

    const client = makeClient();
    const wrapper = makeWrapper(client);

    const { result: r1 } = renderHook(() => useAutomationRuns("auto-1"), { wrapper });
    const { result: r2 } = renderHook(() => useAutomationRuns("auto-2"), { wrapper });

    await waitFor(() => {
      expect(r1.current.loading).toBe(false);
      expect(r2.current.loading).toBe(false);
    });

    expect(r1.current.runs[0].id).toBe("run-1");
    expect(r2.current.runs[0].id).toBe("run-2");

    // Verify distinct cache keys
    expect(client.getQueryData(qk.automations.runs("auto-1"))).toEqual([run1]);
    expect(client.getQueryData(qk.automations.runs("auto-2"))).toEqual([run2]);
  });
});
