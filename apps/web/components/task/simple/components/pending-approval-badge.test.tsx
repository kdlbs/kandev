import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { useEffect } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { agentProfileId as toAgentProfileId, workspaceId as toWorkspaceId } from "@/lib/types/ids";
import {
  PendingApprovalBadge,
  computePendingApprovers,
  pickActiveDecisions,
} from "./pending-approval-badge";
import type { Task, TaskDecision } from "@/app/office/tasks/[id]/types";

afterEach(() => cleanup());

const TS = "2026-05-01T00:00:00Z";

const baseTask: Task = {
  id: "t-1",
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
  approvers: ["agent-1", "agent-2"],
  decisions: [],
  createdBy: "user",
  createdAt: TS,
  updatedAt: TS,
};

function SeedAgents({ agents }: { agents: AgentProfile[] }) {
  const setAgents = useAppStore((s) => s.setOfficeAgentProfiles);
  useEffect(() => setAgents(agents), [setAgents, agents]);
  return null;
}

function makeAgent(id: string, name: string): AgentProfile {
  return {
    id: toAgentProfileId(id),
    workspaceId: toWorkspaceId("ws-1"),
    name,
    agentProfileId: toAgentProfileId("p1"),
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

function Wrapper({ children, agents }: { children: ReactNode; agents: AgentProfile[] }) {
  return (
    <StateProvider>
      <TooltipProvider>
        <SeedAgents agents={agents} />
        {children}
      </TooltipProvider>
    </StateProvider>
  );
}

const approvedDecision: TaskDecision = {
  id: "d1",
  taskId: "t-1",
  deciderType: "agent",
  deciderId: "agent-1",
  deciderName: "CEO",
  role: "approver",
  decision: "approved",
  comment: "",
  createdAt: "2026-05-01T01:00:00Z",
};

describe("computePendingApprovers", () => {
  it("returns the names of approvers without an active approved decision", () => {
    const lookup = { "agent-1": "CEO", "agent-2": "Eng Lead" };
    const pending = computePendingApprovers(
      { approvers: baseTask.approvers, decisions: [approvedDecision] },
      lookup,
    );
    expect(pending).toEqual(["Eng Lead"]);
  });

  it("falls back to the agent id when the lookup is missing the name", () => {
    const pending = computePendingApprovers({ approvers: ["agent-x"], decisions: [] }, {});
    expect(pending).toEqual(["agent-x"]);
  });
});

describe("pickActiveDecisions", () => {
  it("keeps only the most recent decision per (decider, role)", () => {
    const older: TaskDecision = {
      ...approvedDecision,
      id: "old",
      decision: "changes_requested",
      createdAt: "2026-04-01T00:00:00Z",
    };
    const result = pickActiveDecisions([older, approvedDecision]);
    expect(result).toHaveLength(1);
    expect(result[0]?.id).toBe("d1");
  });
});

describe("PendingApprovalBadge", () => {
  it("renders when task is in_review with pending approvers", () => {
    const agents = [makeAgent("agent-1", "CEO"), makeAgent("agent-2", "Eng Lead")];
    render(
      <Wrapper agents={agents}>
        <PendingApprovalBadge task={baseTask} />
      </Wrapper>,
    );
    const badge = screen.getByTestId("pending-approval-badge");
    expect(badge.textContent).toContain("Awaiting approval from 2 agents");
  });

  it("hides when status is not in_review", () => {
    const agents = [makeAgent("agent-1", "CEO"), makeAgent("agent-2", "Eng Lead")];
    const t: Task = { ...baseTask, status: "todo" };
    render(
      <Wrapper agents={agents}>
        <PendingApprovalBadge task={t} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("pending-approval-badge")).toBeNull();
  });

  it("hides when every approver has approved", () => {
    const agents = [makeAgent("agent-1", "CEO"), makeAgent("agent-2", "Eng Lead")];
    const t: Task = {
      ...baseTask,
      decisions: [
        approvedDecision,
        { ...approvedDecision, id: "d2", deciderId: "agent-2", deciderName: "Eng Lead" },
      ],
    };
    render(
      <Wrapper agents={agents}>
        <PendingApprovalBadge task={t} />
      </Wrapper>,
    );
    expect(screen.queryByTestId("pending-approval-badge")).toBeNull();
  });
});
