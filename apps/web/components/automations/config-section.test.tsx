import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockState = {
  workflows: {
    items: [{ id: "workflow-1", name: "Build" }],
  },
  agentProfiles: {
    items: [],
  },
  executors: {
    items: [],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(() => ({
    agentProfiles: [],
    availableAgents: [],
    availableTools: [],
    executors: [],
    settingsAgents: [],
    settingsData: {
      agentsLoaded: true,
      capabilitiesLoaded: true,
      executorsLoaded: true,
    },
  })),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useWorkflows: vi.fn(() => ({ workflows: mockState.workflows.items })),
}));

vi.mock("@/hooks/use-workflow-steps", () => ({
  useWorkflowSteps: vi.fn(() => ({ steps: [] })),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));

vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction: vi.fn().mockResolvedValue({ repositories: [] }),
}));

import { ConfigSection } from "./config-section";

function renderConfigSection(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ConfigSection
        workspaceId="workspace-1"
        workflowId=""
        workflowStepId=""
        agentProfileId=""
        executorProfileId=""
        repositorySelection={{ kind: "none" }}
        executionMode="task"
        conditionType={null}
        onWorkflowChange={() => {}}
        onStepChange={() => {}}
        onAgentProfileChange={() => {}}
        onExecutorProfileChange={() => {}}
        onRepositoryChange={() => {}}
        onExecutionModeChange={() => {}}
        {...overrides}
      />
    </QueryClientProvider>,
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
});
