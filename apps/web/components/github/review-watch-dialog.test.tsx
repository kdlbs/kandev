import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewWatchDialog } from "./review-watch-dialog";

const store = {
  workspaces: { activeId: "ws-1", items: [] },
  workflows: { items: [{ id: "workflow", name: "Delivery", hidden: false }] },
  agentProfiles: { items: [] },
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

afterEach(cleanup);

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
});
