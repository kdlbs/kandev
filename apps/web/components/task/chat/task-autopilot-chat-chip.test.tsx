import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

type MockTask = { id: string; autopilot?: boolean };
type MockState = {
  kanban: { tasks: MockTask[] };
  kanbanMulti: { snapshots: Record<string, { tasks: MockTask[] }> };
};

let mockState: MockState = {
  kanban: { tasks: [] },
  kanbanMulti: { snapshots: {} },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => boolean) => selector(mockState),
}));

import { useTaskAutopilot } from "./task-autopilot-chat-chip";

describe("useTaskAutopilot", () => {
  it("falls back to a snapshot when the direct task omits autopilot", () => {
    mockState = {
      kanban: { tasks: [{ id: "task-1" }] },
      kanbanMulti: { snapshots: { workflow: { tasks: [{ id: "task-1", autopilot: true }] } } },
    };

    const { result } = renderHook(() => useTaskAutopilot("task-1"));

    expect(result.current).toBe(true);
  });

  it("lets an explicit direct false override a stale snapshot", () => {
    mockState = {
      kanban: { tasks: [{ id: "task-1", autopilot: false }] },
      kanbanMulti: { snapshots: { workflow: { tasks: [{ id: "task-1", autopilot: true }] } } },
    };

    const { result } = renderHook(() => useTaskAutopilot("task-1"));

    expect(result.current).toBe(false);
  });
});
