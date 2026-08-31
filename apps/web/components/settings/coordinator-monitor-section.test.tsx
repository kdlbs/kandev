import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CoordinatorMonitorSection } from "./coordinator-monitor-section";
import type { WorkflowStep } from "@/lib/types/http";
import { workflowId } from "@/lib/types/ids";

const steps: WorkflowStep[] = [
  {
    id: "step-spec",
    workflow_id: workflowId("workflow-1"),
    name: "Spec",
    position: 0,
    color: "#0ea5e9",
    created_at: "2026-08-20T00:00:00Z",
    updated_at: "2026-08-20T00:00:00Z",
  },
];

afterEach(cleanup);

describe("CoordinatorMonitorSection", () => {
  it("shows a visible label for each workflow step", () => {
    render(
      <CoordinatorMonitorSection
        workflowId="workflow-1"
        steps={steps}
        config={{}}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Coordinator monitoring Spec")).not.toBeNull();
    expect(screen.getByText("Spec")).not.toBeNull();
  });

  it("keeps step controls disabled while monitoring policy is loading", () => {
    const onChange = vi.fn();
    render(
      <CoordinatorMonitorSection
        workflowId="workflow-1"
        steps={steps}
        config={{}}
        onChange={onChange}
        disabled
      />,
    );

    fireEvent.click(screen.getByLabelText("Coordinator monitoring Spec"));

    expect(screen.getByLabelText("Coordinator monitoring Spec").hasAttribute("disabled")).toBe(
      true,
    );
    expect(onChange).not.toHaveBeenCalled();
  });
});
