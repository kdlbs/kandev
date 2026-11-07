import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClassEligibility } from "@/lib/api/domains/coordinator-automatic-api";
import type { ControlDraft } from "@/lib/coordinators/control-draft";

const eligibilityMock = vi.fn();
const markReviewed = vi.fn();

vi.mock("@/hooks/domains/coordinator/use-action-summary", () => ({
  useActionSummary: () => ({ counts: null, status: "loading", retry: vi.fn() }),
}));
vi.mock("@/hooks/domains/coordinator/use-class-eligibility", () => ({
  useClassEligibility: (...a: unknown[]) => eligibilityMock(...a),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (s: unknown) => unknown) => sel({ auth: { user: { id: "u1" } } }),
}));

import { MayDoSection } from "./may-do-section";

const draft = (createTask: string): ControlDraft =>
  ({
    eligibility: null,
    actions: {
      create_task: createTask,
      start_agent: "denied",
      message: "denied",
      move: "denied",
      resume: "denied",
      stop: "denied",
    },
    watches: { scope: "all", workflowIds: [] },
    projects: null,
  }) as ControlDraft;

const control = (d: ControlDraft) =>
  ({
    draft: d,
    stored: d,
    status: "ready",
    retry: vi.fn(),
    fieldError: null,
    setAction: vi.fn(),
    setWatches: vi.fn(),
    policyDirty: false,
    watchesDirty: false,
    invalid: false,
  }) as never;

const eligibility = (
  eligible: boolean,
  over: Partial<ClassEligibility> = {},
): ClassEligibility => ({
  eligible,
  conditions: [{ name: "volume", met: eligible, value: eligible ? 25 : 19 }],
  setting: "requires_approval",
  changed_by: "",
  changed_at: null,
  ...over,
});

function ready(e: ClassEligibility) {
  eligibilityMock.mockReturnValue({
    eligibility: e,
    status: "ready",
    reload: vi.fn(),
    markReviewed,
    reviewFailed: false,
  });
}

const radio = (action: string) =>
  document.getElementById(`may-do-${action}-automatic`) as HTMLButtonElement;

const renderSection = (createTask: string, canManage = true) =>
  render(
    <MayDoSection
      workspaceId="w1"
      coordinatorId="c1"
      canManage={canManage}
      control={control(draft(createTask))}
      phase3
    />,
  );

beforeEach(() => {
  sessionStorage.clear();
  markReviewed.mockReset();
});
afterEach(cleanup);

describe("MayDoSection with phase 3", () => {
  it("offers start_agent and resume by their own eligibility", () => {
    ready(eligibility(true));
    renderSection("requires_approval");
    expect(radio("start_agent").disabled).toBe(false);
    expect(radio("resume").disabled).toBe(false);
    cleanup();
    ready(eligibility(false));
    renderSection("requires_approval");
    for (const action of ["start_agent", "resume"]) {
      expect(radio(action).disabled).toBe(true);
      expect(screen.getByTestId(`may-do-automatic-note-${action}`).textContent).toContain(
        "Not eligible",
      );
    }
    expect(eligibilityMock).toHaveBeenCalledWith("w1", "c1", "resume", true);
    expect(eligibilityMock).toHaveBeenCalledWith("w1", "c1", "start_agent", true);
  });

  it("disables Automatic with Cannot be raised for the classes that cannot be raised", () => {
    ready(eligibility(true));
    renderSection("requires_approval");
    for (const action of ["message", "move", "stop"]) {
      expect(radio(action).disabled).toBe(true);
      expect(screen.getByTestId(`may-do-automatic-note-${action}`).textContent).toBe(
        "Cannot be raised",
      );
    }
  });

  it("disables create_task Automatic while not eligible", () => {
    ready(eligibility(false));
    renderSection("requires_approval");
    expect(radio("create_task").disabled).toBe(true);
    expect(screen.getByTestId("may-do-automatic-note-create_task").textContent).toContain(
      "Not eligible",
    );
  });

  it("enables create_task Automatic for a manager while eligible", () => {
    ready(eligibility(true));
    renderSection("requires_approval");
    expect(radio("create_task").disabled).toBe(false);
  });

  it("keeps it disabled for a reader even when eligible", () => {
    ready(eligibility(true));
    renderSection("requires_approval", false);
    expect(radio("create_task").disabled).toBe(true);
  });

  it("keeps a raised class selectable and shows the raised record", () => {
    ready(
      eligibility(false, {
        setting: "automatic",
        changed_by: "u1",
        changed_at: "2026-09-01T10:00:00Z",
      }),
    );
    renderSection("automatic");
    expect(radio("create_task").disabled).toBe(false);
    expect(radio("create_task").getAttribute("data-state")).toBe("checked");
    const row = within(screen.getByTestId("may-do-row-create_task"));
    expect(row.getByTestId("automatic-raised-record").textContent).toContain("you");
  });

  it("offers Mark as reviewed only after the review link was opened", () => {
    ready(eligibility(false));
    renderSection("requires_approval");
    const row = within(screen.getByTestId("may-do-row-create_task"));
    const mark = row.getByTestId("automatic-mark-reviewed") as HTMLButtonElement;
    expect(mark.disabled).toBe(true);
    const link = row.getByTestId("may-do-review-create_task");
    link.addEventListener("click", (e) => e.preventDefault());
    fireEvent.click(link);
    expect((row.getByTestId("automatic-mark-reviewed") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(row.getByTestId("automatic-mark-reviewed"));
    expect(markReviewed).toHaveBeenCalledTimes(1);
  });

  it("does not read eligibility when phase 3 is off", () => {
    eligibilityMock.mockReturnValue({
      eligibility: null,
      status: "loading",
      reload: vi.fn(),
      markReviewed,
      reviewFailed: false,
    });
    render(
      <MayDoSection
        workspaceId="w1"
        coordinatorId="c1"
        canManage
        control={control(draft("denied"))}
      />,
    );
    expect(eligibilityMock).toHaveBeenCalledWith("w1", "c1", "create_task", false);
    expect(screen.queryByTestId("automatic-eligibility")).toBeNull();
  });
});
