import { cleanup, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const REPOSITORY_SELECTOR_TEST_ID = "repository-selector";
const REPOSITORY_ROWS_TEST_ID = "repository-rows";

const mockState = {
  workflows: {
    items: [{ id: "workflow-1", name: "Build" }],
  },
  agentProfiles: {
    items: [],
  },
  executors: {
    items: [
      {
        id: "executor-worktree",
        type: "worktree",
        name: "Worktree",
        profiles: [
          {
            id: "profile-worktree",
            executor_id: "executor-worktree",
            name: "Worktree Profile",
          },
        ],
      },
      {
        id: "executor-local",
        type: "local_pc",
        name: "Local PC",
        profiles: [
          {
            id: "profile-local",
            executor_id: "executor-local",
            name: "Local PC Profile",
          },
        ],
      },
    ],
  },
};

const mockRepositories: Array<{ id: string; name: string; local_path: string }> = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useWorkflows: vi.fn(),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: mockRepositories }),
}));

vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction: vi.fn().mockResolvedValue({ repositories: [] }),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn().mockResolvedValue({ steps: [] }),
}));

import { ConfigSection, getExecutorItemDisabledReason } from "./config-section";

function renderConfigSection(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return render(
    <ConfigSection
      workspaceId="workspace-1"
      workflowId=""
      workflowStepId=""
      agentProfileId=""
      executorProfileId=""
      repositorySelections={[]}
      executionMode="task"
      conditionType={null}
      onWorkflowChange={() => {}}
      onStepChange={() => {}}
      onAgentProfileChange={() => {}}
      onExecutorProfileChange={() => {}}
      onRepositoriesChange={() => {}}
      onExecutionModeChange={() => {}}
      {...overrides}
    />,
  );
}

describe("ConfigSection", () => {
  afterEach(cleanup);

  it("marks task workflow fields as required and explains missing selections", () => {
    renderConfigSection();

    screen.getByText("Workflow");
    screen.getByText("Workflow Step");
    expect(screen.getAllByText("required")).toHaveLength(2);
    screen.getByText("Select a workflow to enable saving.");
    screen.getByText("Select a workflow before choosing a step.");
    expect(screen.getByTestId("workflow-selector").getAttribute("aria-describedby")).toBe(
      "workflow-selector-help",
    );
  });

  it("changes step help text once a workflow is selected", () => {
    renderConfigSection({ workflowId: "workflow-1" });

    expect(screen.queryByText("Select a workflow to enable saving.")).toBeNull();
    expect(screen.queryByText("Select a workflow before choosing a step.")).toBeNull();
    screen.getByText("Select a workflow step to enable saving.");
    expect(screen.getByTestId("workflow-step-selector").getAttribute("aria-describedby")).toBe(
      "workflow-step-selector-help",
    );
  });

  it("hides workflow required markers in run mode", () => {
    renderConfigSection({ executionMode: "run" });

    expect(screen.queryByText("Workflow")).toBeNull();
    expect(screen.queryByText("Workflow Step")).toBeNull();
    expect(screen.queryAllByText("required")).toHaveLength(0);
  });

  it("renders a single dropdown when no executor profile is selected", () => {
    renderConfigSection();

    screen.getByTestId(REPOSITORY_SELECTOR_TEST_ID);
    expect(screen.queryByTestId(REPOSITORY_ROWS_TEST_ID)).toBeNull();
  });

  it("renders a repeatable repository list when the executor profile supports multi-repo", () => {
    renderConfigSection({ executorProfileId: "profile-worktree" });

    expect(screen.queryByTestId(REPOSITORY_SELECTOR_TEST_ID)).toBeNull();
    const rows = screen.getByTestId(REPOSITORY_ROWS_TEST_ID);
    within(rows).getByRole("button", { name: "Add repository" });
    screen.getByText(
      "With no repositories selected, this automation runs against the workspace's first repository.",
    );
  });

  it("renders a single dropdown when the executor profile does not support multi-repo", () => {
    renderConfigSection({ executorProfileId: "profile-local" });

    screen.getByTestId(REPOSITORY_SELECTOR_TEST_ID);
    expect(screen.queryByTestId(REPOSITORY_ROWS_TEST_ID)).toBeNull();
  });

  it("keeps the single disabled repository picker for github_pr triggers even with a multi-repo-capable executor", () => {
    renderConfigSection({ executorProfileId: "profile-worktree", conditionType: "github_pr" });

    expect(screen.queryByTestId(REPOSITORY_ROWS_TEST_ID)).toBeNull();
    const selector = screen.getByTestId(REPOSITORY_SELECTOR_TEST_ID);
    expect(selector.hasAttribute("disabled")).toBe(true);
    screen.getByText("PR triggers always use the PR's own repository.");
  });

  it("computes a disabled reason for incompatible executor types once two or more repositories are selected", () => {
    const twoRepos = [
      { kind: "registered" as const, id: "repo-1" },
      { kind: "registered" as const, id: "repo-2" },
    ];
    expect(getExecutorItemDisabledReason("local_pc", twoRepos)).toEqual(
      expect.stringContaining("Local"),
    );
  });

  it("keeps a compatible executor type enabled with two or more repositories selected", () => {
    const twoRepos = [
      { kind: "registered" as const, id: "repo-1" },
      { kind: "registered" as const, id: "repo-2" },
    ];
    expect(getExecutorItemDisabledReason("worktree", twoRepos)).toBeNull();
  });

  it("never disables an executor type when zero or one repository is selected", () => {
    expect(getExecutorItemDisabledReason("local_pc", [])).toBeNull();
    expect(
      getExecutorItemDisabledReason("local_pc", [{ kind: "registered", id: "repo-1" }]),
    ).toBeNull();
  });
});
