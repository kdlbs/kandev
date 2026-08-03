import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { StepDeleteDialog, WorkflowDeleteDialog } from "./workflow-card-dialogs";

/**
 * These dialogs used to build their descriptions by concatenation — an English
 * `s` for the plural and a leading-space suffix for the unsaved-changes hint.
 * Both are now catalog messages (`count` + `_one`/`_other`, and a joined
 * sentence), so the shapes that would silently regress are pinned here:
 *
 *   - the singular / plural task counts in each of the three step variants,
 *   - the space between the base sentence and the unsaved-changes hint,
 *   - the step name arriving as an interpolated value rather than baked into
 *     the message (steps are renamed freely, so it is user data).
 */

const STEP_NAME = "In Review";

const NO_WORKFLOWS: Workflow[] = [];
const NO_STEPS: WorkflowStep[] = [];

// Only `id` and `name` are read (the migration <Select>), but the fixture
// satisfies the whole contract so a field added to WorkflowStep fails here
// rather than being silently absent behind an `as` cast.
function step(id: string, name: string): WorkflowStep {
  return {
    id,
    workflow_id: "workflow-1" as WorkflowStep["workflow_id"],
    name,
    position: 0,
    color: "bg-slate-500",
    created_at: "",
    updated_at: "",
  };
}

function renderWorkflowDelete(taskCount: number | null, hasUnsavedChanges = false) {
  render(
    <WorkflowDeleteDialog
      open
      onOpenChange={vi.fn()}
      workflowTaskCount={taskCount}
      otherWorkflows={NO_WORKFLOWS}
      targetWorkflowId=""
      setTargetWorkflowId={vi.fn()}
      targetWorkflowSteps={NO_STEPS}
      targetStepId=""
      setTargetStepId={vi.fn()}
      migrateLoading={false}
      deleteLoading={false}
      onDelete={vi.fn()}
      onMigrateAndDelete={vi.fn()}
      hasUnsavedChanges={hasUnsavedChanges}
    />,
  );
}

function renderStepDelete(
  stepTaskCount: number | null,
  stepsForMigration: WorkflowStep[] = [],
  hasUnsavedChanges = false,
) {
  render(
    <StepDeleteDialog
      open
      onOpenChange={vi.fn()}
      stepName={STEP_NAME}
      stepTaskCount={stepTaskCount}
      stepsForMigration={stepsForMigration}
      targetStep=""
      setTargetStep={vi.fn()}
      loading={false}
      pending={false}
      onMigrateAndDelete={vi.fn()}
      onDeleteAndTasks={vi.fn()}
      hasUnsavedChanges={hasUnsavedChanges}
    />,
  );
}

const migrationTarget = [step("step-2", "Done")];

describe("WorkflowDeleteDialog description", () => {
  afterEach(cleanup);

  it("uses the singular task form for exactly one task", () => {
    renderWorkflowDelete(1);
    expect(
      screen.getByText(
        "This workflow has 1 task. Choose where to migrate them, or delete the workflow and archive the tasks.",
      ),
    ).toBeTruthy();
  });

  it("uses the plural task form for more than one task", () => {
    renderWorkflowDelete(3);
    expect(
      screen.getByText(
        "This workflow has 3 tasks. Choose where to migrate them, or delete the workflow and archive the tasks.",
      ),
    ).toBeTruthy();
  });

  it("falls back to the no-tasks sentence and appends the unsaved-changes hint with a space", () => {
    renderWorkflowDelete(0, true);
    expect(
      screen.getByText(
        "This will permanently delete the workflow and all its steps. Unsaved workflow changes will be discarded.",
      ),
    ).toBeTruthy();
  });
});

describe("StepDeleteDialog description", () => {
  afterEach(cleanup);

  it("names the step without tasks", () => {
    renderStepDelete(0);
    expect(
      screen.getByText(`This will permanently delete the ${STEP_NAME} workflow step.`),
    ).toBeTruthy();
  });

  it("inflects the migration sentence on the task count", () => {
    renderStepDelete(1, migrationTarget);
    expect(
      screen.getByText(
        `${STEP_NAME} has 1 task. Choose where to migrate them, or delete the step and its tasks.`,
      ),
    ).toBeTruthy();
    cleanup();
    renderStepDelete(2, migrationTarget);
    expect(
      screen.getByText(
        `${STEP_NAME} has 2 tasks. Choose where to migrate them, or delete the step and its tasks.`,
      ),
    ).toBeTruthy();
  });

  it("inflects the no-migration-target sentence on the task count", () => {
    renderStepDelete(1);
    expect(screen.getByText(`Deleting ${STEP_NAME} will also affect its 1 task.`)).toBeTruthy();
    cleanup();
    renderStepDelete(4);
    expect(screen.getByText(`Deleting ${STEP_NAME} will also affect its 4 tasks.`)).toBeTruthy();
  });

  it("appends the unsaved-step hint as a separate sentence", () => {
    renderStepDelete(2, migrationTarget, true);
    expect(
      screen.getByText(
        `${STEP_NAME} has 2 tasks. Choose where to migrate them, or delete the step and its tasks. Unsaved step changes will be discarded.`,
      ),
    ).toBeTruthy();
  });
});
