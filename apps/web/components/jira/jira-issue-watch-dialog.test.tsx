import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { JiraIssueWatchDialog } from "./jira-issue-watch-dialog";

const promptEditor = vi.hoisted(() => vi.fn());

const store = {
  workspaces: { activeId: "ws-1", items: [{ id: "ws-1", name: "Workspace" }] },
  workflows: { items: [{ id: "workflow", name: "Delivery", hidden: false }] },
  agentProfiles: { items: [] },
  executors: { items: [] },
  features: { dynamicAgentRouting: false },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof store) => unknown) => selector(store),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/use-workflows", () => ({ useWorkflows: vi.fn() }));
vi.mock("@/hooks/use-workflow-steps", () => ({
  useWorkflowSteps: () => ({ steps: [{ id: "step", name: "Implement" }], loading: false }),
  stepPlaceholder: () => "Select step",
}));
vi.mock("@/components/watcher-repository-fields", () => ({
  WatcherRepositoryFields: () => <div>Repository and base branch</div>,
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div aria-label={props.ariaLabel as string} data-testid={props.testId as string} />;
  },
}));

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("JiraIssueWatchDialog", () => {
  it("uses Jira placeholders with saved-prompt references", () => {
    render(
      <JiraIssueWatchDialog
        open
        onOpenChange={vi.fn()}
        watch={null}
        workspaceId="ws-1"
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    expect(promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        testId: "jira-issue-watch-prompt-editor",
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "issue.key" })]),
      }),
    );
  });
});
