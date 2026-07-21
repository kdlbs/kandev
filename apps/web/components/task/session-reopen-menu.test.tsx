import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent } from "@kandev/ui/dropdown-menu";
import type { TaskSession } from "@/lib/types/http";

type MockSessionInput = {
  id: string;
  task_id?: string;
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown>;
  started_at?: string;
  workflow_step_id?: string | null;
  state?: string;
  is_primary?: boolean;
  snapshotLabel?: string;
};

let mockSessions: TaskSession[] = [];

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: (_taskId: string | null) => ({ sessions: mockSessions }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({
      agentProfiles: {
        items: [
          { id: "p1", label: "Agent One", agent_name: "agent-one" },
          { id: "p2", label: "Agent Two", agent_name: "agent-two" },
        ],
      },
      kanban: {
        steps: [
          { id: "spec", position: 0, title: "Spec" },
          { id: "work", position: 1, title: "Work" },
        ],
        tasks: [{ id: "t1", primarySessionId: null }],
      },
      kanbanMulti: { snapshots: {} },
    }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (s: unknown) => unknown) =>
    selector({ api: { getPanel: () => null }, centerGroupId: "center" }),
}));

import { SessionReopenMenuItems } from "./session-reopen-menu";

// Test fixtures deliberately use plain strings for id/agent ids rather than
// the app's branded id types (TaskId/SessionId/AgentProfileId) — cast once
// here instead of threading brands through every fixture field.
const makeSession = (overrides: MockSessionInput): TaskSession =>
  ({
    id: overrides.id,
    task_id: overrides.task_id ?? "t1",
    agent_profile_id: overrides.agent_profile_id,
    agent_profile_snapshot: overrides.agent_profile_snapshot ?? {
      label: overrides.snapshotLabel ?? "Snapshot",
    },
    started_at: overrides.started_at ?? "2025-01-01T00:00:00Z",
    workflow_step_id: overrides.workflow_step_id ?? null,
    state: overrides.state ?? "COMPLETED",
    is_primary: overrides.is_primary ?? false,
  }) as unknown as TaskSession;

describe("SessionReopenMenuItems step-flow ordering", () => {
  afterEach(() => {
    cleanup();
    mockSessions = [];
  });

  it("renders reopen items in step-flow order and labels include rank", () => {
    // Create sessions in work-first order so sorting must reorder them
    const sWork = makeSession({
      id: "s2",
      workflow_step_id: "work",
      started_at: "2025-01-01T00:00:00Z",
      agent_profile_id: "p2",
    });
    const sSpec = makeSession({
      id: "s1",
      workflow_step_id: "spec",
      started_at: "2025-01-02T00:00:00Z",
      agent_profile_id: "p1",
    });
    // supply unsorted list
    mockSessions = [sWork, sSpec];

    render(
      <DropdownMenu open modal={false}>
        <DropdownMenuContent forceMount>
          <SessionReopenMenuItems taskId={"t1"} />
        </DropdownMenuContent>
      </DropdownMenu>,
    );

    // DropdownMenuContent portals into document.body, so query there rather
    // than the render() container. Items render with
    // data-testid="reopen-session-<id>" in DOM order.
    const items = Array.from(
      document.body.querySelectorAll('[data-testid^="reopen-session-"]'),
    ) as HTMLElement[];
    expect(items.map((el) => el.getAttribute("data-testid"))).toEqual([
      "reopen-session-s1",
      "reopen-session-s2",
    ]);

    // Labels should include step title + rank (Spec #1, Work #2), rendered
    // exactly once per row — not duplicated by a separate rank chip.
    const texts = items.map((el) => el.textContent ?? "");
    expect(texts[0]).toBe("Spec #1");
    expect(texts[1]).toBe("Work #2");
  });
});
