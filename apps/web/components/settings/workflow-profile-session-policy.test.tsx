import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workflow, WorkflowProfileSessionPolicy } from "@/lib/types/http";
import { WorkflowProfileSessionPolicyField } from "./workflow-profile-session-policy";

afterEach(cleanup);

function renderField(
  value: WorkflowProfileSessionPolicy | undefined = "complete",
  savedValue: WorkflowProfileSessionPolicy | undefined = value,
  readOnly = false,
) {
  const onChange = vi.fn();
  const workflow = { id: "workflow-1", profile_session_policy: value } as Workflow;
  const savedWorkflow =
    savedValue === undefined
      ? undefined
      : ({ id: "workflow-1", profile_session_policy: savedValue } as Workflow);
  render(
    <WorkflowProfileSessionPolicyField
      workflow={workflow}
      savedWorkflow={savedWorkflow}
      onChange={onChange}
      readOnly={readOnly}
    />,
  );
  return onChange;
}

describe("WorkflowProfileSessionPolicyField", () => {
  it("shows the selected policy and its explanation", () => {
    renderField("park_reuse", "complete");

    const trigger = screen.getByTestId("workflow-profile-session-policy-select");
    expect(trigger.getAttribute("data-settings-dirty")).toBe("true");
    expect(trigger.textContent).toContain("Park and reuse the previous session");
    expect(
      screen.getByText(/Stop the old runtime but keep its conversation available/),
    ).toBeTruthy();
  });

  it("offers all policies and reports the canonical value", () => {
    const onChange = renderField();
    fireEvent.click(screen.getByTestId("workflow-profile-session-policy-select"));

    expect(screen.getByRole("option", { name: /Complete the previous session/ })).toBeTruthy();
    expect(
      screen.getByRole("option", { name: /Park and reuse the previous session/ }),
    ).toBeTruthy();
    const parkNew = screen.getByRole("option", { name: /Park and start a new session/ });
    expect(parkNew).toBeTruthy();

    fireEvent.click(parkNew);
    expect(onChange).toHaveBeenCalledWith("park_new");
  });

  it("keeps the selected policy readable while synced workflows are read-only", () => {
    renderField("park_new", "park_new", true);

    const trigger = screen.getByTestId("workflow-profile-session-policy-select");
    expect((trigger as HTMLButtonElement).disabled).toBe(true);
    expect(trigger.textContent).toContain("Park and start a new session");
    expect(screen.getByText(/Each return to the profile starts a fresh conversation/)).toBeTruthy();
  });
});
