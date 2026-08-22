import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IssueWatchDialog } from "./issue-watch-dialog";

const promptEditor = vi.hoisted(() => vi.fn());

const store = {
  workspaces: { activeId: "ws-1", items: [] },
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
vi.mock("@/components/github/repo-filter-selector", () => ({
  RepoFilterSelector: () => <div>Repository filters</div>,
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div data-testid="issue-watch-prompt-editor" />;
  },
}));

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("IssueWatchDialog", () => {
  it("uses the shared editor with GitHub issue placeholders and saved prompts", () => {
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
        testId: "github-issue-watch-prompt-editor",
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "issue.title" })]),
      }),
    );
  });
});
