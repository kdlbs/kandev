import { cleanup, render, screen } from "@testing-library/react";
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
  useSettingsData: vi.fn(),
}));

vi.mock("@/hooks/use-workflows", () => ({
  useWorkflows: vi.fn(),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));

vi.mock("@/app/actions/workspaces", () => ({
  discoverRepositoriesAction: vi.fn().mockResolvedValue({ repositories: [] }),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn().mockResolvedValue({ steps: [] }),
}));

import { ConfigSection } from "./config-section";

function renderConfigSection(overrides: Partial<ComponentProps<typeof ConfigSection>> = {}) {
  return render(
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
    />,
  );
}

describe("ConfigSection", () => {
  afterEach(cleanup);

  it("marks task workflow fields as required and explains missing selections", () => {
    renderConfigSection();

    expect(screen.getByText("Workflow")).toBeTruthy();
    expect(screen.getByText("Workflow Step")).toBeTruthy();
    expect(screen.getAllByText("required")).toHaveLength(2);
    expect(screen.getByText("Select a workflow to enable saving.")).toBeTruthy();
    expect(screen.getByText("Select a workflow before choosing a step.")).toBeTruthy();
  });
});
