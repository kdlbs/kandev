import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const implementPlanMock = vi.fn().mockResolvedValue(undefined);
vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  useImplementPlanRunner: () => implementPlanMock,
}));

vi.mock("./task-plan-revisions", () => ({
  TaskPlanRevisions: () => null,
}));

vi.mock("./chat/implement-plan-button", () => ({
  ImplementPlanButton: ({
    onClick,
    disabled,
  }: {
    onClick: (fresh: boolean) => void;
    disabled?: boolean;
  }) => (
    <button
      type="button"
      data-testid="implement-button"
      disabled={disabled}
      onClick={() => onClick(false)}
    >
      Implement
    </button>
  ),
}));

import { PlanPanelHeader } from "./task-plan-panel-header";

const TASK_1 = "task-1";
const REVISIONS: TaskPlanRevision[] = [];

function renderHeader(attemptSave: (content: string, title?: string) => Promise<TaskPlan | null>) {
  return render(
    <PlanPanelHeader
      taskId={TASK_1}
      plan={null}
      draftContent="draft that will be sent"
      hasUnsavedChanges={true}
      activeSessionId="session-1"
      revisions={REVISIONS}
      isLoadingRevisions={false}
      isSaving={false}
      isAgentBusy={false}
      attemptSave={attemptSave}
      onOpenRevisions={() => {}}
      onRevert={async () => null}
      loadRevisionContent={async () => ""}
      previewRevisionId={null}
      setPreviewRevision={() => {}}
      comparePair={[null, null]}
      toggleCompareSelection={() => {}}
      clearComparePair={() => {}}
    />,
  );
}

describe("PlanPanelHeader Implement routes through the guarded save entry point", () => {
  afterEach(() => {
    cleanup();
    implementPlanMock.mockClear();
  });

  it("calls the attemptSave prop, not a separate raw save path, before implementing", async () => {
    // Regression test: handleImplement used to call a raw savePlan prop that
    // bypassed usePlanDraft's attemptSave wrapper, so an oversized draft
    // rejected via the Implement button was never recorded in the autosave
    // suppression ref — the very next autosave tick resubmitted the same
    // rejected content. Routing through attemptSave (whatever the caller
    // passes as this prop) closes that gap; this test pins that the panel's
    // wiring passes the guarded callback and that a rejection here does not
    // proceed to implement.
    const attemptSave = vi.fn().mockResolvedValue(null); // simulates a size rejection
    renderHeader(attemptSave);

    fireEvent.click(screen.getByTestId("implement-button"));

    await waitFor(() =>
      expect(attemptSave).toHaveBeenCalledWith("draft that will be sent", undefined),
    );
    expect(implementPlanMock).not.toHaveBeenCalled();
  });

  it("proceeds to implement once the guarded save succeeds", async () => {
    const savedPlan: TaskPlan = {
      task_id: TASK_1,
      content: "draft that will be sent",
      title: "Plan",
    } as TaskPlan;
    const attemptSave = vi.fn().mockResolvedValue(savedPlan);
    renderHeader(attemptSave);

    fireEvent.click(screen.getByTestId("implement-button"));

    await waitFor(() => expect(implementPlanMock).toHaveBeenCalledWith(false));
    expect(attemptSave).toHaveBeenCalledWith("draft that will be sent", undefined);
  });
});
