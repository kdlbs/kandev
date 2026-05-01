import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WorkflowSection } from "./task-create-dialog-form-body";

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
