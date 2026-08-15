import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type MockWorkflow = { id: string; name: string; workspaceId: string };

const WORKFLOW_SELECTOR = "workflow-selector";
const FEATURE_DEV = "Feature Dev";
const WORKSPACE_1 = "workspace-1";
const FETCH_FAILED = "Failed to fetch";
const LOCAL_WORKFLOW_ID = "local-1";

// Real workflows always carry a workspace_id; the store maps it through as
// workspaceId. Omitting it here would let a workspace-scoping bug pass.
const LOCAL_WORKFLOW: MockWorkflow = {
  id: "workflow-1",
  name: "Build",
  workspaceId: WORKSPACE_1,
};

const REPOSITORY_SELECTOR_TEST_ID = "repository-selector";
const REPOSITORY_ROWS_TEST_ID = "repository-rows";

const mockState = {
  features: { dynamicAgentRouting: true },
  workflows: {
    items: [LOCAL_WORKFLOW] as MockWorkflow[],
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

// The editor fetches its own workflows rather than reading the shared store
// slot, so the seam under test is this call — including which workspace it
// asks for.
const mockListWorkflows = vi.fn();
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
}));

import { ConfigSection, getExecutorItemDisabledReason } from "./config-section";

// Every render triggers the fetch, so it needs a resolvable default or the
// effect throws before the test gets to its own assertion.
beforeEach(() => {
  mockListWorkflows.mockResolvedValue({ workflows: [] });
});

function configSection(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return (
    <ConfigSection
      workspaceId={WORKSPACE_1}
      workflowId=""
      workflowStepId=""
      agentProfileId=""
      executorProfileId=""
      repositorySelections={[]}
      conditionType={null}
      onWorkflowChange={() => {}}
      onStepChange={() => {}}
      onAgentProfileChange={() => {}}
      onExecutorProfileChange={() => {}}
      onRepositoriesChange={() => {}}
      {...overrides}
    />
  );
}

function renderConfigSection(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return render(configSection(overrides));
}

describe("ConfigSection", () => {
  afterEach(() => {
    cleanup();
    mockState.workflows.items = [LOCAL_WORKFLOW];
  });

  it("offers the workflow fields without demanding either of them", () => {
    renderConfigSection();

    screen.getByText("Workflow");
    screen.getByText("Workflow Step");
    // Both are optional now: an automation that only reports has no place on a
    // board, so nothing here may claim it blocks saving.
    expect(screen.queryAllByText("required")).toHaveLength(0);
    expect(screen.getByTestId(WORKFLOW_SELECTOR).getAttribute("aria-invalid")).toBeNull();
    expect(screen.getByTestId("workflow-step-selector").getAttribute("aria-invalid")).toBeNull();
    expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).toContain("optional");
  });

  it("keeps the step field disabled and explained until a workflow is picked", () => {
    renderConfigSection();

    screen.getByText("Select a workflow before choosing a step.");
    expect(screen.getByTestId("workflow-step-selector").getAttribute("aria-describedby")).toBe(
      "workflow-step-selector-help",
    );
  });

  it("drops the step hint once a workflow is selected", () => {
    renderConfigSection({ workflowId: "workflow-1" });

    expect(screen.queryByText("Select a workflow before choosing a step.")).toBeNull();
    expect(
      screen.getByTestId("workflow-step-selector").getAttribute("aria-describedby"),
    ).toBeNull();
  });

  it("no longer offers an execution mode to choose", () => {
    renderConfigSection();

    expect(screen.queryByTestId("execution-mode-selector")).toBeNull();
    expect(screen.queryByText("Execution Mode")).toBeNull();
  });
});

describe("ConfigSection workflow scoping", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("asks for the workflows of the workspace being edited", async () => {
    mockListWorkflows.mockResolvedValue({ workflows: [] });

    renderConfigSection({ workspaceId: WORKSPACE_1 });

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalled());
    expect(mockListWorkflows.mock.calls[0][0]).toBe(WORKSPACE_1);
  });

  it("offers the workflows that workspace returned", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV, workspace_id: WORKSPACE_1 }],
    });

    renderConfigSection({ workspaceId: WORKSPACE_1, workflowId: LOCAL_WORKFLOW_ID });

    await waitFor(() =>
      expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).toContain(FEATURE_DEV),
    );
  });

  it("never shows a workflow the shared store happens to hold for another workspace", async () => {
    // The store slot is global and races between the active workspace and this
    // page. Reading it here once offered another workspace's workflows, and a
    // name present in both makes the wrong one look right — so an automation
    // could be saved against a workflow its workspace does not own.
    mockState.workflows.items = [
      { id: "foreign-1", name: FEATURE_DEV, workspaceId: "workspace-other" },
    ];
    mockListWorkflows.mockResolvedValue({ workflows: [] });

    renderConfigSection({ workspaceId: WORKSPACE_1, workflowId: "foreign-1" });

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalled());
    expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).not.toContain(FEATURE_DEV);
  });

  it("refetches when the workspace changes", async () => {
    mockListWorkflows.mockResolvedValue({ workflows: [] });

    const { rerender } = renderConfigSection({ workspaceId: WORKSPACE_1 });
    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledTimes(1));

    rerender(configSection({ workspaceId: "workspace-2" }));

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalledTimes(2));
    expect(mockListWorkflows.mock.calls[1][0]).toBe("workspace-2");
  });
});

describe("ConfigSection workflow load failures", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  it("retries a failed fetch rather than leaving the field empty forever", async () => {
    // A page opened while the backend is restarting gets "Failed to fetch".
    // One attempt would strand the field for the rest of the session.
    mockListWorkflows
      .mockRejectedValueOnce(new TypeError(FETCH_FAILED))
      .mockResolvedValue({ workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV }] });

    renderConfigSection({ workspaceId: WORKSPACE_1, workflowId: LOCAL_WORKFLOW_ID });

    await waitFor(
      () => expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).toContain(FEATURE_DEV),
      { timeout: 3000 },
    );
    expect(mockListWorkflows.mock.calls.length).toBeGreaterThan(1);
  });

  it("says so and offers a retry once the attempts are exhausted", async () => {
    // Fake timers so the test doesn't sit through the real backoff.
    vi.useFakeTimers();
    mockListWorkflows.mockRejectedValue(new TypeError(FETCH_FAILED));

    renderConfigSection({ workspaceId: WORKSPACE_1 });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    const error = screen.getByTestId("workflow-load-error");
    expect(error.textContent).toContain("Couldn't load workflows");
    expect(screen.getByTestId("workflow-retry")).toBeTruthy();
  });

  it("refetches when the retry is used", async () => {
    vi.useFakeTimers();
    mockListWorkflows.mockRejectedValue(new TypeError(FETCH_FAILED));

    renderConfigSection({ workspaceId: WORKSPACE_1 });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    screen.getByTestId("workflow-retry");

    const before = mockListWorkflows.mock.calls.length;
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV }],
    });
    fireEvent.click(screen.getByTestId("workflow-retry"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });

    expect(mockListWorkflows.mock.calls.length).toBeGreaterThan(before);
    expect(screen.queryByTestId("workflow-load-error")).toBeNull();
  });

  it("keeps a list it already had when a later fetch fails", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV }],
    });

    const { rerender } = renderConfigSection({
      workspaceId: WORKSPACE_1,
      workflowId: LOCAL_WORKFLOW_ID,
    });
    await waitFor(() =>
      expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).toContain(FEATURE_DEV),
    );

    // Blanking a usable field on a flake is worse than showing stale options.
    mockListWorkflows.mockRejectedValue(new TypeError(FETCH_FAILED));
    rerender(configSection({ workspaceId: WORKSPACE_1, workflowId: LOCAL_WORKFLOW_ID }));

    expect(screen.getByTestId(WORKFLOW_SELECTOR).textContent).toContain(FEATURE_DEV);
  });
});

// Workflow and step are optional now, and optional has to be reversible: an
// automation upgraded from the era when a workflow was mandatory can only be
// freed of one if the picker offers a way to say "none".
describe("ConfigSection clearing the workflow", () => {
  afterEach(cleanup);

  it("offers a None entry in both the workflow and step pickers", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV, workspace_id: WORKSPACE_1 }],
    });
    renderConfigSection({ workflowId: LOCAL_WORKFLOW_ID });

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalled());
    fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR));
    await screen.findByRole("option", { name: "No workflow" });
  });

  it("reports a cleared workflow as an empty id, not the sentinel", async () => {
    mockListWorkflows.mockResolvedValue({
      workflows: [{ id: LOCAL_WORKFLOW_ID, name: FEATURE_DEV, workspace_id: WORKSPACE_1 }],
    });
    const onWorkflowChange = vi.fn();
    renderConfigSection({ workflowId: LOCAL_WORKFLOW_ID, onWorkflowChange });

    await waitFor(() => expect(mockListWorkflows).toHaveBeenCalled());
    fireEvent.click(screen.getByTestId(WORKFLOW_SELECTOR));
    fireEvent.click(await screen.findByRole("option", { name: "No workflow" }));

    // The sentinel exists only because Radix refuses an empty option value; it
    // must never reach the form.
    expect(onWorkflowChange).toHaveBeenCalledWith("");
  });
});

describe("ConfigSection repository and executor pickers", () => {
  afterEach(cleanup);

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
