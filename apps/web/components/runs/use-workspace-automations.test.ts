import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockList = vi.fn();
vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: (...args: unknown[]) => mockList(...args),
}));

import { useWorkspaceAutomations } from "./use-workspace-automations";
import type { Automation } from "@/lib/types/automation";

const WORKSPACE = "ws-1";
const OTHER_WORKSPACE = "ws-2";

function mkAutomation(id: string, workspaceId = WORKSPACE): Automation {
  return {
    id,
    workspace_id: workspaceId,
    name: id,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    triggers: [],
  };
}

beforeEach(() => {
  mockList.mockResolvedValue([]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useWorkspaceAutomations", () => {
  it("loads the given workspace's automations", async () => {
    mockList.mockResolvedValue([mkAutomation("a1")]);

    const { result } = renderHook(() => useWorkspaceAutomations(WORKSPACE));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).toHaveBeenCalledWith(WORKSPACE);
    expect(result.current.automations.map((a) => a.id)).toEqual(["a1"]);
    expect(result.current.error).toBeNull();
  });

  it("does not call the API without a workspace", async () => {
    const { result } = renderHook(() => useWorkspaceAutomations(undefined));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).not.toHaveBeenCalled();
    expect(result.current.automations).toEqual([]);
  });

  it("does not call the API for an inactive navigation surface", async () => {
    const { result } = renderHook(() => useWorkspaceAutomations(WORKSPACE, { active: false }));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockList).not.toHaveBeenCalled();
    expect(result.current.automations).toEqual([]);
  });

  it("surfaces a load failure instead of an empty list", async () => {
    mockList.mockRejectedValue(new Error("ws down"));

    const { result } = renderHook(() => useWorkspaceAutomations(WORKSPACE));

    await waitFor(() => expect(result.current.error).toBe("ws down"));
    expect(result.current.loading).toBe(false);
    expect(result.current.automations).toEqual([]);
  });

  it("recovers on refresh after a failure", async () => {
    mockList.mockRejectedValueOnce(new Error("ws down")).mockResolvedValue([mkAutomation("a2")]);

    const { result } = renderHook(() => useWorkspaceAutomations(WORKSPACE));
    await waitFor(() => expect(result.current.error).toBe("ws down"));

    act(() => result.current.refresh());

    await waitFor(() => expect(result.current.automations.map((a) => a.id)).toEqual(["a2"]));
    expect(result.current.error).toBeNull();
  });

  it("never shows the previous workspace's automations under a new one", async () => {
    mockList.mockResolvedValue([mkAutomation("a1")]);
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useWorkspaceAutomations(workspaceId),
      { initialProps: { workspaceId: WORKSPACE } },
    );
    await waitFor(() => expect(result.current.automations.map((a) => a.id)).toEqual(["a1"]));

    // Never resolves, so the render right after the switch is the one under
    // test: the old workspace's rows must already be gone.
    mockList.mockReturnValue(new Promise(() => {}));
    rerender({ workspaceId: OTHER_WORKSPACE });

    expect(result.current.automations).toEqual([]);
    expect(result.current.loading).toBe(true);
  });

  it("drops a response that arrives after the workspace already changed", async () => {
    let resolveFirst: ((value: Automation[]) => void) | undefined;
    mockList.mockImplementationOnce(
      () =>
        new Promise<Automation[]>((resolve) => {
          resolveFirst = resolve;
        }),
    );

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useWorkspaceAutomations(workspaceId),
      { initialProps: { workspaceId: WORKSPACE } },
    );

    mockList.mockResolvedValue([mkAutomation("b1", OTHER_WORKSPACE)]);
    rerender({ workspaceId: OTHER_WORKSPACE });
    await waitFor(() => expect(result.current.automations.map((a) => a.id)).toEqual(["b1"]));

    // The first workspace's slow response lands last. It must not win.
    await act(async () => {
      resolveFirst?.([mkAutomation("a1")]);
    });

    expect(result.current.automations.map((a) => a.id)).toEqual(["b1"]);
  });
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.11
it.each(["success", "failure"])(
  "ignores obsolete %s after a newer empty refresh",
  async (outcome) => {
    mockList.mockResolvedValue([mkAutomation("baseline")]);
    const { result } = renderHook(() => useWorkspaceAutomations(WORKSPACE));
    await waitFor(() => expect(result.current.loading).toBe(false));
    let resolve!: (rows: Automation[]) => void;
    let reject!: (error: Error) => void;
    const old = new Promise<Automation[]>((yes, no) => {
      resolve = yes;
      reject = no;
    });
    mockList.mockReturnValueOnce(old).mockResolvedValueOnce([]);
    act(() => result.current.refresh());
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.automations).toEqual([]);
    await act(async () => {
      if (outcome === "success") resolve([mkAutomation("obsolete")]);
      else reject(new Error("obsolete failure"));
    });
    expect(result.current.automations).toEqual([]);
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  },
);

// @covers AC-OFFICE-AUTOMATION-RUNS-001.11
it("masks every B render when returning to A before B settles", async () => {
  const seen: Array<{ workspace: string; ids: string[] }> = [];
  mockList.mockResolvedValue([mkAutomation("a")]);
  const { result, rerender } = renderHook(
    ({ workspace }) => {
      const value = useWorkspaceAutomations(workspace);
      seen.push({ workspace, ids: value.automations.map((item) => item.id) });
      return value;
    },
    { initialProps: { workspace: WORKSPACE } },
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  let resolve!: (rows: Automation[]) => void;
  const pending = new Promise<Automation[]>((yes) => {
    resolve = yes;
  });
  mockList.mockReturnValueOnce(pending).mockResolvedValue([mkAutomation("a-current")]);
  rerender({ workspace: OTHER_WORKSPACE });
  rerender({ workspace: WORKSPACE });
  await waitFor(() =>
    expect(result.current.automations.map((item) => item.id)).toEqual(["a-current"]),
  );
  await act(async () => resolve([mkAutomation("b", OTHER_WORKSPACE)]));
  expect(
    seen
      .filter((value) => value.workspace === WORKSPACE)
      .every((value) => !value.ids.includes("b")),
  ).toBe(true);
  expect(
    seen
      .filter((value) => value.workspace === OTHER_WORKSPACE)
      .every((value) => value.ids.length === 0),
  ).toBe(true);
  expect(result.current.automations.map((item) => item.id)).toEqual(["a-current"]);
});
