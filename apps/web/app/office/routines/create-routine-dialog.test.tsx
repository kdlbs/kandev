import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent, within } from "@testing-library/react";
import type { AgentProfile } from "@/lib/state/slices/office/types";

import { CreateRoutineDialog } from "./create-routine-dialog";

afterEach(() => cleanup());

const AGENT = {
  id: "agent-1",
  workspaceId: "ws-1",
  name: "Worker",
  role: "worker",
  status: "idle",
  agentProfileId: "profile-1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
} as AgentProfile;

function renderDialog() {
  return render(
    <CreateRoutineDialog open onOpenChange={vi.fn()} agents={[AGENT]} onSubmit={vi.fn()} />,
  );
}

// Advances from step 0 (Details) to step 2 (Schedule), where the catch-up
// policy control and the conditional catch_up_max input live. Step 0
// requires a name and an assignee before "Next" is enabled; step 1 (task
// template) has no required fields.
function goToScheduleStep() {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Nightly digest" } });
  fireEvent.click(screen.getAllByRole("combobox")[0]);
  fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "Worker" }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
}

// AC-OFFICE-ROUTINE-CATCHUP-003.8: the catch_up_max control renders under
// the summarizing policy (the create dialog's default) and disappears
// under skip_missed. Regression coverage for the enqueue_missed_with_cap
// -> summarize_missed rename, which silently drops this control if only
// the option values/defaults are updated without the "===" guard deciding
// whether the input renders at all.
describe("CreateRoutineDialog catch-up max control (AC-003.8)", () => {
  it("renders the catch-up max input by default (summarize_missed)", () => {
    renderDialog();
    goToScheduleStep();
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();
    expect(screen.getByRole("spinbutton")).toBeTruthy();
  });

  it("hides the catch-up max input after selecting skip missed", () => {
    renderDialog();
    goToScheduleStep();
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();

    // Comboboxes on the Schedule step, in DOM order: trigger kind,
    // concurrency policy, catch-up policy.
    const catchUpPolicyCombobox = screen.getAllByRole("combobox")[2];
    fireEvent.click(catchUpPolicyCombobox);
    fireEvent.click(
      within(screen.getByRole("listbox")).getByRole("option", { name: /skip missed/i }),
    );

    expect(screen.queryByText(/catch-up max/i)).toBeNull();
    expect(screen.queryByRole("spinbutton")).toBeNull();
  });

  it("submits summarize_missed (not the retired enqueue_missed_with_cap value) as the default policy", () => {
    const onSubmit = vi.fn();
    render(
      <CreateRoutineDialog open onOpenChange={vi.fn()} agents={[AGENT]} onSubmit={onSubmit} />,
    );
    goToScheduleStep();
    fireEvent.change(screen.getByPlaceholderText("0 9 * * *"), { target: { value: "0 9 * * *" } });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ catchUpPolicy: "summarize_missed" }),
    );
  });
});
