import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/app/office/tasks/[id]/types";
import { TaskProperties } from "./task-properties";

const mockAppState = vi.hoisted(() => ({
  auth: {
    mode: "enabled",
    authenticated: true,
    user: { id: "user-1" } as { id: string } | null,
    ssoProviders: [],
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockAppState) => unknown) => selector(mockAppState),
}));

vi.mock("./components/status-picker", () => ({ StatusPicker: () => null }));
vi.mock("./components/priority-picker", () => ({ PriorityPicker: () => null }));
vi.mock("./components/labels-picker", () => ({ LabelsPicker: () => null }));
vi.mock("./components/assignee-picker", () => ({ AssigneePicker: () => null }));
vi.mock("./components/human-assignee-picker", () => ({
  HumanAssigneePicker: () => <span data-testid="human-assignee-picker-trigger" />,
}));
vi.mock("./components/project-picker", () => ({ ProjectPicker: () => null }));
vi.mock("./components/parent-picker", () => ({ ParentPicker: () => null }));
vi.mock("./components/blockers-picker", () => ({ BlockersPicker: () => null }));
vi.mock("./components/sub-issues-row", () => ({ SubIssuesRow: () => null }));
vi.mock("./components/reviewers-picker", () => ({ ReviewersPicker: () => null }));
vi.mock("./components/approvers-picker", () => ({ ApproversPicker: () => null }));
vi.mock("./components/pending-approval-badge", () => ({ PendingApprovalBadge: () => null }));
vi.mock("./components/quorum-status-badge", () => ({ QuorumStatusBadge: () => null }));

const TASK = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "TASK-1",
  title: "Auth gate task",
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user-1",
  createdAt: "2026-09-05T12:00:00Z",
  updatedAt: "2026-09-05T12:00:00Z",
} satisfies Task;

afterEach(() => {
  cleanup();
  mockAppState.auth.mode = "enabled";
  mockAppState.auth.user = { id: "user-1" };
});

describe("TaskProperties human assignee", () => {
  it.each(["disabled", "setup"] as const)("hides the row in %s auth mode", (mode) => {
    mockAppState.auth.mode = mode;
    mockAppState.auth.user = { id: "default-user" };

    render(<TaskProperties task={TASK} />);

    expect(screen.queryByTestId("human-assignee-picker-trigger")).toBeNull();
  });

  it("hides the row when enabled auth has no signed-in user", () => {
    mockAppState.auth.user = null;

    render(<TaskProperties task={TASK} />);

    expect(screen.queryByTestId("human-assignee-picker-trigger")).toBeNull();
  });

  it("shows the row when authentication is enabled for a signed-in user", () => {
    render(<TaskProperties task={TASK} />);

    expect(screen.getByTestId("human-assignee-picker-trigger")).toBeTruthy();
  });
});
