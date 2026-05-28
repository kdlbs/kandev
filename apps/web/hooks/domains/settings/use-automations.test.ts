import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Automation } from "@/lib/types/automation";

// ---------------------------------------------------------------------------
// Mock the automation API so tests don't hit real WebSocket calls
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

import {
  listAutomations,
  createAutomation,
  updateAutomation as apiUpdateAutomation,
  deleteAutomation,
  enableAutomation,
  disableAutomation,
  triggerAutomation,
} from "@/lib/api/domains/automation-api";

const mockListAutomations = vi.mocked(listAutomations);
const mockCreateAutomation = vi.mocked(createAutomation);
const mockUpdateAutomation = vi.mocked(apiUpdateAutomation);
const mockDeleteAutomation = vi.mocked(deleteAutomation);
const mockEnableAutomation = vi.mocked(enableAutomation);
const mockDisableAutomation = vi.mocked(disableAutomation);
const mockTriggerAutomation = vi.mocked(triggerAutomation);

import { useAutomations } from "./use-automations";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAutomation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: "auto-1",
    workspace_id: "ws-1",
    name: "Test Automation",
    description: "",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    agent_profile_id: "ap-1",
    executor_profile_id: "ep-1",
    repository_id: "repo-1",
    prompt: "",
    task_title_template: "",
    execution_mode: "task",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    triggers: [],
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

async function setupLoaded(workspaceId: string, items: Automation[]) {
  mockListAutomations.mockResolvedValue(items);
  const client = makeClient();
  const { result } = renderHook(() => useAutomations(workspaceId), {
    wrapper: makeWrapper(client),
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  return { result, client };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.resetAllMocks();
});

describe("useAutomations — fetch", () => {
  it("returns empty items and loading=true initially", () => {
    mockListAutomations.mockReturnValue(new Promise(() => {}));
    const client = makeClient();

    const { result } = renderHook(() => useAutomations("ws-1"), {
      wrapper: makeWrapper(client),
    });

    expect(result.current.items).toEqual([]);
    expect(result.current.loading).toBe(true);
  });

  it("returns loaded automations after successful fetch", async () => {
    const auto = makeAutomation();
    const { result } = await setupLoaded("ws-1", [auto]);
    expect(result.current.items).toEqual([auto]);
  });

  it("does not fetch when workspaceId is null", () => {
    const client = makeClient();
    renderHook(() => useAutomations(null), { wrapper: makeWrapper(client) });
    expect(mockListAutomations).not.toHaveBeenCalled();
  });

  it("uses separate cache entries for different workspaces", async () => {
    const autoWs1 = makeAutomation({ id: "a1", workspace_id: "ws-1" });
    const autoWs2 = makeAutomation({ id: "a2", workspace_id: "ws-2" });
    mockListAutomations.mockImplementation((wsId: string) =>
      wsId === "ws-1" ? Promise.resolve([autoWs1]) : Promise.resolve([autoWs2]),
    );

    const client = makeClient();
    const wrapper = makeWrapper(client);
    const { result: r1 } = renderHook(() => useAutomations("ws-1"), { wrapper });
    const { result: r2 } = renderHook(() => useAutomations("ws-2"), { wrapper });

    await waitFor(() => {
      expect(r1.current.loading).toBe(false);
      expect(r2.current.loading).toBe(false);
    });

    expect(r1.current.items[0].id).toBe("a1");
    expect(r2.current.items[0].id).toBe("a2");
    expect(client.getQueryData(qk.automations.list("ws-1"))).toEqual([autoWs1]);
    expect(client.getQueryData(qk.automations.list("ws-2"))).toEqual([autoWs2]);
  });
});

describe("useAutomations — mutations", () => {
  it("create() calls API and invalidates the list", async () => {
    const existing = makeAutomation({ id: "auto-1" });
    const created = makeAutomation({ id: "auto-2", name: "New" });
    const { result } = await setupLoaded("ws-1", [existing]);

    mockCreateAutomation.mockResolvedValue({ ...created, webhook_secret: "whsec-test" });
    mockListAutomations.mockResolvedValue([existing, created]);

    await act(async () => {
      await result.current.create({
        workspace_id: "ws-1",
        name: "New",
        workflow_id: "wf-1",
        workflow_step_id: "step-1",
        agent_profile_id: "ap-1",
        executor_profile_id: "ep-1",
      });
    });

    expect(mockCreateAutomation).toHaveBeenCalledOnce();
    await waitFor(() => expect(result.current.items).toHaveLength(2));
  });

  it("update() patches the automation in the cache", async () => {
    const original = makeAutomation({ id: "auto-1", name: "Original" });
    const updated = makeAutomation({ id: "auto-1", name: "Updated" });
    mockUpdateAutomation.mockResolvedValue(updated);
    const { result } = await setupLoaded("ws-1", [original]);

    await act(async () => {
      await result.current.update("auto-1", { name: "Updated" });
    });

    await waitFor(() => expect(result.current.items[0].name).toBe("Updated"));
  });

  it("remove() removes the automation from the cache", async () => {
    const auto = makeAutomation({ id: "auto-1" });
    mockDeleteAutomation.mockResolvedValue(undefined);
    const { result } = await setupLoaded("ws-1", [auto]);

    expect(result.current.items).toHaveLength(1);

    await act(async () => {
      await result.current.remove("auto-1");
    });

    await waitFor(() => expect(result.current.items).toHaveLength(0));
  });

  it("enable() updates the automation in the cache", async () => {
    const disabledAuto = makeAutomation({ id: "auto-1", enabled: false });
    const enabledAuto = makeAutomation({ id: "auto-1", enabled: true });
    mockEnableAutomation.mockResolvedValue(enabledAuto);
    const { result } = await setupLoaded("ws-1", [disabledAuto]);

    await act(async () => {
      await result.current.enable("auto-1");
    });

    await waitFor(() => expect(result.current.items[0].enabled).toBe(true));
  });

  it("disable() updates the automation in the cache", async () => {
    const active = makeAutomation({ id: "auto-1", enabled: true });
    const inactive = makeAutomation({ id: "auto-1", enabled: false });
    mockDisableAutomation.mockResolvedValue(inactive);
    const { result } = await setupLoaded("ws-1", [active]);

    await act(async () => {
      await result.current.disable("auto-1");
    });

    await waitFor(() => expect(result.current.items[0].enabled).toBe(false));
  });

  it("trigger() calls the trigger API", async () => {
    mockTriggerAutomation.mockResolvedValue({ triggered: true });
    const { result } = await setupLoaded("ws-1", []);

    await act(async () => {
      await result.current.trigger("auto-1");
    });

    expect(mockTriggerAutomation).toHaveBeenCalledWith("auto-1");
  });

  it("refresh() invalidates the list query", async () => {
    const { result } = await setupLoaded("ws-1", []);
    mockListAutomations.mockResolvedValue([makeAutomation()]);

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(mockListAutomations).toHaveBeenCalledTimes(2);
  });
});
