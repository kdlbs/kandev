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
  auth: { user: null },
  workspaces: { items: [] },
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
  settingsAgents: {
    items: [
      {
        id: "agent-1",
        name: "codex",
        profiles: [
          {
            id: "agent-1",
            name: "Default",
            agentId: "agent-1",
            agentDisplayName: "Codex",
            model: "",
            allowIndexing: false,
            autoApprove: true,
            cliFlags: [],
            cliPassthrough: false,
            envVars: [{ key: "SERVICE_KEY", secret_id: "profile-secret" }],
          },
        ],
      },
    ],
  },
  secrets: { items: [], loaded: true, loading: false },
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
  useAppStoreApi: () => null,
  useOptionalAppStoreApi: () => null,
  useOptionalAppStore: (_selector: unknown, fallback: unknown) => fallback,
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
  getRepositoryDiscoveryAction: vi.fn().mockResolvedValue({ repositories: [] }),
  refreshRepositoryDiscoveryAction: vi.fn().mockResolvedValue({ repositories: [] }),
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
    showDiscoveryControls,
  }: {
    rows: Array<{ key: string; branch: string }>;
    onAdd: () => void;
    showDiscoveryControls?: boolean;
  }) => (
    <div data-testid="shared-repository-chips">
      {rows.map((row) => (
        <span key={row.key}>{row.branch}</span>
      ))}
      <button type="button" onClick={onAdd}>
        Add repository
      </button>
      {showDiscoveryControls ? <div data-testid="repository-discovery-selector-enabled" /> : null}
    </div>
  ),
}));

import {
  ConfigSection,
  getExecutorItemDisabledReason,
  supportsPersonalCredentialChoices,
  WorkflowField,
} from "./config-section";

function renderConfig(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return render(
    <ConfigSection
      workspaceId={WORKSPACE_ID}
      automation={null}
      onAutomationChange={() => {}}
      agentProfileId=""
      executorProfileId=""
      credentialChoicesEligible
      repositorySelections={[]}
      personalCredentialBindings={[]}
      onPersonalCredentialBindingsChange={() => {}}
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
    render(
      <WorkflowField
        workspaceId={WORKSPACE_ID}
        workflowId=""
        taskMode="automation_run"
        onWorkflowChange={() => {}}
      />,
    );

    fireEvent.click(screen.getByTestId("workflow-selector-trigger"));

    expect(screen.getByText("Development")).toBeTruthy();
    expect(screen.getByText("Backlog")).toBeTruthy();
    expect(screen.getByTestId("workflow-agent-logo")).toBeTruthy();
    expect(screen.getByTestId("step-agent-logo")).toBeTruthy();
    expect(screen.queryByText("Workflow Step")).toBeNull();
    expect(screen.queryByTestId("workflow-step-selector")).toBeNull();
  });

  it("requires a workflow for a normal task and explains why", () => {
    render(
      <WorkflowField
        workspaceId={WORKSPACE_ID}
        workflowId=""
        taskMode="normal_task"
        onWorkflowChange={() => {}}
      />,
    );
    expect(
      screen.getByText("Choose a workflow so this task appears in Kanban and the sidebar."),
    ).toBeTruthy();
  });

  it("keeps a workflow optional for automation history", () => {
    render(
      <WorkflowField
        workspaceId={WORKSPACE_ID}
        workflowId=""
        taskMode="automation_run"
        onWorkflowChange={() => {}}
      />,
    );
    expect(
      screen.queryByText("Choose a workflow so this task appears in Kanban and the sidebar."),
    ).toBeNull();
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

  it("shows credential fields from the full agent profile snapshot", () => {
    renderConfig({ agentProfileId: "agent-1" });

    expect(screen.getByTestId("automation-personal-credential-SERVICE_KEY")).toBeTruthy();
  });

  it("hides personal secret selection when the draft uses unsupported settings", () => {
    const unsupportedDraft = {
      agentProfileId: "agent-1",
      credentialChoicesEligible: false,
    } as unknown as Partial<ComponentProps<typeof ConfigSection>>;
    renderConfig(unsupportedDraft);

    expect(screen.queryByTestId("automation-personal-credential-fields")).toBeNull();
  });

  it("uses paired repository chips and has no workspace-default choice", () => {
    renderConfig();

    expect(screen.getByTestId("shared-repository-chips")).toBeTruthy();
    expect(screen.getByTestId("repository-discovery-selector-enabled")).toBeTruthy();
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

describe("supportsPersonalCredentialChoices", () => {
  it.each([
    ["manual", { type: "manual" }],
    ["scheduled", { type: "scheduled" }],
  ] as const)("allows the %s trigger for a new automation-history task", (_, trigger) => {
    expect(supportsPersonalCredentialChoices("automation_run", "new_task", [trigger])).toBe(true);
  });

  it.each([
    ["continuing a thread", "automation_run", "reuse_thread", [{ type: "scheduled" }]],
    ["webhook delivery", "automation_run", "new_task", [{ type: "webhook" }]],
    ["provider events", "automation_run", "new_task", [{ type: "github_pr" }]],
    ["managed conversations", "managed_conversation", "new_task", [{ type: "scheduled" }]],
  ] as const)("rejects credential choices for %s", (_, taskMode, continuation, triggers) => {
    expect(
      supportsPersonalCredentialChoices(
        taskMode,
        continuation,
        triggers as unknown as { type: "manual" | "scheduled" | "webhook" | "github_pr" }[],
      ),
    ).toBe(false);
  });

  it("allows credentials when Run now is the only trigger", () => {
    expect(supportsPersonalCredentialChoices("automation_run", "new_task", [])).toBe(true);
  });
});
