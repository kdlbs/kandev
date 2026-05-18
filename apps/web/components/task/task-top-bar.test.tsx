import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
  PRTopbarButton: () => null,
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

vi.mock("@/components/task/topbar-action-overflow", () => ({
  TopbarActionOverflow: ({ items }: { items: Array<{ id: string; content: React.ReactNode }> }) => (
    <div>
      {items.map((item) => (
        <div key={item.id}>{item.content}</div>
      ))}
    </div>
  ),
}));

vi.mock("@/components/integrations/integrations-menu", () => ({
  IntegrationsMenu: () => null,
}));

vi.mock("@/components/task/branch-path-popover", () => ({
  BranchPathPopover: () => null,
}));

describe("TaskTopBar executor environment controls", () => {
  it("hides the executor environment button for filesystem executors", () => {
    render(<TaskTopBar taskId="task-1" remoteExecutorType="worktree" />);

    expect(screen.queryByTestId("executor-settings-button")).toBeNull();
  });

  it("shows the executor environment button for Docker executors", () => {
    render(<TaskTopBar taskId="task-1" remoteExecutorType="local_docker" />);

    expect(screen.getByTestId("executor-settings-button")).toBeTruthy();
  });
});
