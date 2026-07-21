import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IssueWatchDialog } from "./issue-watch-dialog";

const store = {
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

afterEach(cleanup);

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
});
