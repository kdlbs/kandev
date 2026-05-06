import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { useEffect } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { ApproversPicker } from "./approvers-picker";
import { buildDecisionLookup } from "./agents-multi-picker";
import type { Task, TaskDecision } from "@/app/office/tasks/[id]/types";

afterEach(() => cleanup());

const TS = "2026-05-01T00:00:00Z";

function makeAgent(id: string, name: string): AgentProfile {
  return {
    id,
    workspaceId: "ws-1",
    name,
    agentProfileId: "p1",
    role: "worker",
    icon: "🤖",
    status: "idle",
    reportsTo: "",
    permissions: {},
    budgetMonthlyCents: 0,
    maxConcurrentSessions: 1,
    desiredSkills: [],
    executorPreference: {},
    pauseReason: "",
    billingType: "api_key",
    utilization: null,
    agentId: "claude",
    agentDisplayName: "Claude",
    model: "claude-sonnet-4-5",
    allowIndexing: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: TS,
    updatedAt: TS,
  };
}

function SeedAgents({ agents }: { agents: AgentProfile[] }) {
  const setAgents = useAppStore((s) => s.setOfficeAgentProfiles);
  useEffect(() => setAgents(agents), [setAgents, agents]);
  return null;
}

function Wrapper({
  children,
  task,
  agents,
}: {
  children: ReactNode;
  task: Task;
  agents: AgentProfile[];
}) {
  const ctx = { task, applyPatch: () => {}, restore: () => {} };
  return (
    <StateProvider>
      <TooltipProvider>
        <SeedAgents agents={agents} />
        <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
      </TooltipProvider>
    </StateProvider>
  );
}

const ROLE_APPROVER = "approver" as const;
const TASK_ID = "t-1";

const baseTask: Task = {
  id: TASK_ID,
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "in_review",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: ["a-approved", "a-changes", "a-pending"],
  decisions: [
    {
      id: "d1",
      taskId: TASK_ID,
      deciderType: "agent",
      deciderId: "a-approved",
      deciderName: "Alice",
      role: ROLE_APPROVER,
      decision: "approved",
      comment: "",
      createdAt: "2026-05-01T01:00:00Z",
    },
    {
      id: "d2",
      taskId: TASK_ID,
      deciderType: "agent",
      deciderId: "a-changes",
      deciderName: "Bob",
      role: ROLE_APPROVER,
      decision: "changes_requested",
      comment: "fix tests",
      createdAt: "2026-05-01T02:00:00Z",
    },
  ],
  createdBy: "user",
  createdAt: TS,
  updatedAt: TS,
};

describe("buildDecisionLookup", () => {
  it("keeps the most recent decision per (decider, role)", () => {
    const decisions: TaskDecision[] = [
      {
        id: "old",
        taskId: TASK_ID,
        deciderType: "agent",
        deciderId: "x",
        deciderName: "X",
        role: ROLE_APPROVER,
        decision: "approved",
        comment: "",
        createdAt: "2026-05-01T00:00:00Z",
      },
      {
        id: "new",
        taskId: TASK_ID,
        deciderType: "agent",
        deciderId: "x",
        deciderName: "X",
        role: ROLE_APPROVER,
        decision: "changes_requested",
        comment: "",
        createdAt: "2026-05-02T00:00:00Z",
      },
    ];
    const map = buildDecisionLookup(decisions, "approver");
    expect(map.get("x")?.id).toBe("new");
  });

  it("filters by role", () => {
    const decisions: TaskDecision[] = [
      {
        id: "rev",
        taskId: TASK_ID,
        deciderType: "agent",
        deciderId: "x",
        deciderName: "X",
        role: "reviewer",
        decision: "approved",
        comment: "",
        createdAt: TS,
      },
    ];
    expect(buildDecisionLookup(decisions, ROLE_APPROVER).size).toBe(0);
    expect(buildDecisionLookup(decisions, "reviewer").size).toBe(1);
  });
});

describe("ApproversPicker chip decoration", () => {
  it("renders a status icon per chip matching the most recent decision", () => {
    const agents = [
      makeAgent("a-approved", "Alice"),
      makeAgent("a-changes", "Bob"),
      makeAgent("a-pending", "Carol"),
    ];
    render(
      <Wrapper task={baseTask} agents={agents}>
        <ApproversPicker task={baseTask} />
      </Wrapper>,
    );
    expect(screen.getAllByTestId("decision-icon-approved")).toHaveLength(1);
    expect(screen.getAllByTestId("decision-icon-changes_requested")).toHaveLength(1);
    expect(screen.getAllByTestId("decision-icon-pending")).toHaveLength(1);
  });
});
