import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CreateEditSelectors, WorkflowSection } from "./task-create-dialog-form-body";

vi.mock("@/components/workflow-selector-row", () => ({
  WorkflowSelectorRow: ({ selectedWorkflowId }: { selectedWorkflowId: string | null }) => (
    <button type="button">Workflow selector {selectedWorkflowId ?? "none"}</button>
  ),
}));

const workflow = { id: "wf-1", name: "Development" };

function renderWorkflowSection(effectiveWorkflowId: string | null) {
  return render(
    <WorkflowSection
      isCreateMode={true}
      isTaskStarted={false}
      workflows={[workflow]}
      snapshots={{}}
      effectiveWorkflowId={effectiveWorkflowId}
      onWorkflowChange={() => {}}
      agentProfiles={[]}
    />,
  );
}

describe("WorkflowSection", () => {
  it("keeps the selector reachable when no effective workflow is selected", () => {
    renderWorkflowSection(null);

    expect(screen.getByRole("button", { name: /workflow selector none/i })).toBeTruthy();
  });

  it("does not show redundant selector for a selected single workflow without overrides", () => {
    const { container } = renderWorkflowSection("wf-1");

    expect(container.textContent).toBe("");
  });
});

describe("CreateEditSelectors", () => {
  it("links credential setup to the selected executor profile", () => {
    const EmptySelector = () => <button type="button">selector</button>;

    render(
      <CreateEditSelectors
        isTaskStarted={false}
        agentProfiles={[{ id: "agent-1", label: "Codex", agent_name: "codex" } as never]}
        agentProfilesLoading={false}
        agentProfileOptions={[]}
        agentProfileId=""
        onAgentProfileChange={() => {}}
        isCreatingSession={false}
        executorProfileOptions={[]}
        executorProfileId="exec-profile-1"
        onExecutorProfileChange={() => {}}
        executorsLoading={false}
        AgentSelectorComponent={EmptySelector}
        ExecutorProfileSelectorComponent={EmptySelector}
        workflowAgentLocked={false}
        noCompatibleAgent={true}
        executorProfileName="Docker"
      />,
    );

    expect(screen.getByRole("link", { name: /configure credentials/i }).getAttribute("href")).toBe(
      "/settings/executors/exec-profile-1",
    );
  });
});
