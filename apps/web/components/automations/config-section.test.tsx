import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const WORKSPACE_ID = "workspace-1";
const mockRepositories = [
  {
    id: "repo-1",
    name: "kandev",
    local_path: "/code/kandev",
    provider_owner: "",
    provider_name: "",
  },
];

const mockState = {
  workflows: {
    items: [
      {
        id: "workflow-1",
        name: "Development",
        workspaceId: WORKSPACE_ID,
        agent_profile_id: "agent-1",
      },
    ],
  },
  kanbanMulti: {
    snapshots: {
      "workflow-1": {
        workflowId: "workflow-1",
        workflowName: "Development",
        steps: [
          {
            id: "step-backlog",
            title: "Backlog",
            position: 0,
            color: "#123456",
            is_start_step: true,
            agent_profile_id: "agent-1",
          },
        ],
        tasks: [],
      },
    },
  },
  agentProfiles: {
    items: [
      {
        id: "agent-1",
        label: "Codex • Default",
        agent_name: "codex",
      },
    ],
  },
  executors: {
    items: [
      {
        id: "executor-worktree",
        type: "worktree",
        name: "Worktree",
        profiles: [{ id: "worktree-1", name: "Worktree", executor_type: "worktree" }],
      },
      {
        id: "executor-local",
        type: "local_pc",
        name: "Local",
        profiles: [{ id: "local-1", name: "Local", executor_type: "local_pc" }],
      },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useWorkflows: () => ({ workflows: mockState.workflows.items }),
}));

vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: vi.fn(),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: mockRepositories }),
}));

vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction: vi.fn().mockResolvedValue({ repositories: [] }),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: (profiles: Array<{ id: string; label: string }>) =>
    profiles.map((profile) => ({
      value: profile.id,
      label: profile.label,
      renderLabel: () => <span data-testid="shared-agent-logo">{profile.label}</span>,
    })),
  useExecutorProfileOptions: (profiles: Array<{ id: string; name: string }>) =>
    profiles.map((profile) => ({
      value: profile.id,
      label: profile.name,
      renderLabel: () => <span data-testid="shared-executor-logo">{profile.name}</span>,
    })),
}));

vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  WorkspaceRepoChips: ({
    rows,
    onAdd,
  }: {
    rows: Array<{ key: string; branch: string }>;
    onAdd: () => void;
  }) => (
    <div data-testid="shared-repository-chips">
      {rows.map((row) => (
        <span key={row.key}>{row.branch}</span>
      ))}
      <button type="button" onClick={onAdd}>
        Add repository
      </button>
    </div>
  ),
}));

import { ConfigSection, getExecutorItemDisabledReason } from "./config-section";

function renderConfig(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return render(
    <ConfigSection
      workspaceId={WORKSPACE_ID}
      workflowId=""
      agentProfileId=""
      executorProfileId=""
      repositorySelections={[]}
      onWorkflowChange={() => {}}
      onAgentProfileChange={() => {}}
      onExecutorProfileChange={() => {}}
      onRepositoriesChange={() => {}}
      {...overrides}
    />,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ConfigSection shared task selectors", () => {
  it("shows the shared workflow preview and removes the workflow-step picker", () => {
    renderConfig();

    fireEvent.click(screen.getByTestId("workflow-selector-trigger"));

    expect(screen.getByText("Development")).toBeTruthy();
    expect(screen.getByText("Backlog")).toBeTruthy();
    expect(screen.getByTestId("workflow-agent-logo")).toBeTruthy();
    expect(screen.getByTestId("step-agent-logo")).toBeTruthy();
    expect(screen.queryByText("Workflow Step")).toBeNull();
    expect(screen.queryByTestId("workflow-step-selector")).toBeNull();
  });

  it("uses the shared searchable profile selectors", async () => {
    renderConfig();

    fireEvent.click(screen.getByTestId("agent-profile-selector"));
    expect(screen.getByTestId("shared-agent-logo")).toBeTruthy();
    expect(screen.getByPlaceholderText("Search agents...")).toBeTruthy();

    fireEvent.click(screen.getByTestId("executor-profile-selector"));
    expect(screen.getAllByTestId("shared-executor-logo")).toHaveLength(2);
    expect(screen.getByPlaceholderText("Search profiles...")).toBeTruthy();
  });

  it("uses paired repository chips and has no workspace-default choice", () => {
    renderConfig();

    expect(screen.getByTestId("shared-repository-chips")).toBeTruthy();
    expect(screen.queryByText("Use workspace default")).toBeNull();
    expect(
      screen.getByText("Run without repository files in a task-owned scratch workspace."),
    ).toBeTruthy();
  });

  it("adds an empty repository row through the shared chip control", () => {
    const onRepositoriesChange = vi.fn();
    renderConfig({ onRepositoriesChange });

    fireEvent.click(screen.getByRole("button", { name: "Add repository" }));

    expect(onRepositoriesChange).toHaveBeenCalledWith([
      expect.objectContaining({ kind: "none", branch: "" }),
    ]);
  });
});

describe("getExecutorItemDisabledReason", () => {
  it("allows Worktree to use a repository-free scratch workspace", () => {
    expect(getExecutorItemDisabledReason("worktree", [])).toBeNull();
  });

  it("uses the shared multi-repository capability guard", () => {
    expect(
      getExecutorItemDisabledReason("local_pc", [
        { kind: "registered", id: "repo-1", branch: "main" },
        { kind: "registered", id: "repo-2", branch: "main" },
      ]),
    ).not.toBeNull();
  });
});
