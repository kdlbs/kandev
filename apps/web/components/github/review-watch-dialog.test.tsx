import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewWatchDialog } from "./review-watch-dialog";

const promptEditor = vi.hoisted(() => vi.fn());

const store = {
  workspaces: { activeId: "ws-1", items: [] },
  workflows: { items: [{ id: "workflow", name: "Delivery", hidden: false }] },
  agentProfiles: { items: [] },
  features: { dynamicAgentRouting: true },
  executors: { items: [] },
  prompts: { items: [], loaded: true, loading: false },
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
vi.mock("@/components/github/repo-filter-selector", () => ({
  RepoFilterSelector: () => <div>Repository filters</div>,
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div data-testid="review-watch-prompt-editor" />;
  },
}));

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("ReviewWatchDialog", () => {
  it("explains lifecycle retention for Auto and the Always delete override", () => {
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

    expect(screen.getByText(/user engagement or enabled PR lifecycle prompts/i)).not.toBeNull();
    const cleanupField = screen.getByText("Cleanup behavior").parentElement;
    const cleanupSelect = cleanupField?.querySelector("button");
    if (!cleanupSelect) throw new Error("Cleanup policy selector was not rendered");
    fireEvent.click(cleanupSelect);
    fireEvent.click(screen.getByRole("option", { name: "Always delete" }));
    expect(screen.getByText(/Always delete.*overrides retention/i)).not.toBeNull();
  });

  it("uses the shared editor with placeholders and saved-prompt references", () => {
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
        testId: "github-review-watch-prompt-editor",
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "pr.title" })]),
      }),
    );
  });
});
