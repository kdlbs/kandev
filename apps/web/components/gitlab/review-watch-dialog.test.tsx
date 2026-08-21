import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewWatchDialog } from "./review-watch-dialog";

const promptEditor = vi.hoisted(() => vi.fn());

const store = {
  features: { dynamicAgentRouting: false },
  workspaces: { activeId: "ws-1" },
  workflows: { items: [{ id: "workflow", name: "Delivery", hidden: false }] },
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

describe("ReviewWatchDialog", () => {
  it("renders the complete watch controls in a narrow-safe dialog", () => {
    render(
      <ReviewWatchDialog
        open
        onOpenChange={vi.fn()}
        watch={null}
        workspaceId="ws-1"
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("Project paths")).toBeTruthy();
    expect(screen.getByLabelText("Maximum in-flight tasks")).toBeTruthy();
    expect(screen.getByLabelText("Task prompt")).toBeTruthy();
    expect(screen.getByLabelText("Workflow")).toBeTruthy();
    expect(screen.getByLabelText("Agent profile")).toBeTruthy();
    expect(
      screen.getByText(/leave empty to match merge requests requesting your review/i),
    ).toBeTruthy();
  });

  it("uses GitLab merge request placeholders and saved-prompt references", () => {
    render(
      <ReviewWatchDialog
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
        testId: "review-watch-prompt-editor",
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "mr.url" })]),
      }),
    );
  });
});
