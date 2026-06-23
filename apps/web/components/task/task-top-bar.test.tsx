import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TaskTopBar } from "./task-top-bar";

afterEach(() => cleanup());

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/hooks/domains/session/use-session-git", () => ({
  useSessionGit: () => ({ branch: "", renameBranch: vi.fn() }),
}));

vi.mock("@/components/task/executor-settings-button", () => ({
  ExecutorSettingsButton: () => <button data-testid="executor-settings-button">executor</button>,
}));

vi.mock("@/components/task/port-forward-dialog", () => ({
  PortForwardButton: () => <button>ports</button>,
}));

vi.mock("@/components/task/document/document-controls", () => ({
  DocumentControls: () => null,
}));

vi.mock("@/components/vcs-split-button", () => ({
  VcsSplitButton: () => <button>vcs</button>,
}));

vi.mock("@/components/github/pr-topbar-button", () => ({
  PRTopbarButton: () => <button data-testid="pr-topbar-button">#1472</button>,
}));

vi.mock("@/components/gitlab/mr-topbar-button", () => ({
  MRTopbarButton: () => null,
}));

vi.mock("@/components/jira/jira-ticket-button", () => ({
  JiraTicketButton: () => null,
  extractJiraKey: () => null,
}));

vi.mock("@/components/jira/jira-link-button", () => ({
  JiraLinkButton: () => null,
}));

vi.mock("@/components/linear/linear-issue-button", () => ({
  LinearIssueButton: () => null,
  extractLinearKey: () => null,
}));

vi.mock("@/components/linear/linear-link-button", () => ({
  LinearLinkButton: () => null,
}));

vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAvailable: () => false,
}));

vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAvailable: () => false,
}));

vi.mock("@/components/task/workflow-stepper", () => ({
  WorkflowStepper: () => null,
}));

vi.mock("@/components/task/layout-preset-selector", () => ({
  LayoutPresetSelector: () => null,
}));

vi.mock("@/components/task/editors-menu", () => ({
  EditorsMenu: () => null,
}));

vi.mock("@/components/task/quick-chat-button", () => ({
  QuickChatButton: () => null,
}));

vi.mock("@/components/integrations/integrations-menu", () => ({
  IntegrationsMenu: () => null,
}));

vi.mock("@/components/task/branch-path-popover", () => ({
  BranchPathPopover: () => null,
}));

describe("TaskTopBar executor environment controls", () => {
  it("hides the executor environment button for filesystem executors", () => {
    renderTopBar(<TaskTopBar taskId="task-1" remoteExecutorType="worktree" />);

    expect(screen.queryByTestId("executor-settings-button")).toBeNull();
  });

  it("shows the executor environment button for Docker executors", () => {
    renderTopBar(<TaskTopBar taskId="task-1" remoteExecutorType="local_docker" />);

    expect(screen.getByTestId("executor-settings-button")).toBeTruthy();
  });
});

describe("TaskTopBar GitHub issue link", () => {
  it("shows the linked issue before linked pull requests", () => {
    renderTopBar(
      <TaskTopBar
        taskId="task-1"
        issueUrl="https://github.com/kdlbs/kandev/issues/1470"
        issueNumber={1470}
      />,
    );

    const issue = screen.getByTestId("issue-topbar-button");
    const pr = screen.getByTestId("pr-topbar-button");

    expect(issue.textContent).toContain("#1470");
    expect(issue.compareDocumentPosition(pr) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

function renderTopBar(ui: React.ReactNode) {
  return render(<StateProvider>{ui}</StateProvider>);
}
