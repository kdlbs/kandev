import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IssueWatchDialog } from "./issue-watch-dialog";

const promptEditor = vi.hoisted(() => vi.fn());

const store = {
  features: { dynamicAgentRouting: false },
  workspaces: { activeId: "ws-1" },
  workflows: { items: [] },
  agentProfiles: { items: [] },
  executors: { items: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof store) => unknown) => selector(store),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/use-workflows", () => ({ useWorkflows: vi.fn() }));
vi.mock("@/hooks/use-workflow-steps", () => ({
  useWorkflowSteps: () => ({ steps: [], loading: false }),
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

describe("IssueWatchDialog", () => {
  it("renders labels and GitLab query controls", () => {
    render(
      <IssueWatchDialog
        open
        onOpenChange={vi.fn()}
        watch={null}
        workspaceId="ws-1"
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Labels")).toBeTruthy();
    expect(screen.getByLabelText("GitLab query parameters")).toBeTruthy();
  });

  it("uses GitLab issue placeholders and saved-prompt references", () => {
    render(
      <IssueWatchDialog
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
        testId: "issue-watch-prompt-editor",
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "issue.url" })]),
      }),
    );
  });
});
