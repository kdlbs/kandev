import { describe, it, expect } from "vitest";
import { computeDisabledReason } from "./task-create-dialog-footer";
import type { TaskCreateDialogFooterProps } from "./task-create-dialog-footer";

const REASON_TITLE = "Add a task title";
const REASON_REPO = "Select a repository";
const REASON_BRANCH = "Select a branch";
const REASON_WORKSPACE = "Select a workspace";
const REASON_WORKFLOW = "Select a workflow";
const REASON_AGENT = "Select an agent";
const KIND_START = "start-task" as const;
const KIND_UPDATE = "update" as const;
const KIND_DEFAULT = "default" as const;

function makeProps(
  overrides: Partial<TaskCreateDialogFooterProps> = {},
): TaskCreateDialogFooterProps {
  return {
    isSessionMode: false,
    isCreateMode: true,
    isEditMode: false,
    isTaskStarted: false,
    isPassthroughProfile: false,
    isCreatingSession: false,
    isCreatingTask: false,
    hasTitle: true,
    hasDescription: true,
    hasRepositorySelection: true,
    branch: "main",
    agentProfileId: "agent-1",
    workspaceId: "ws-1",
    effectiveWorkflowId: "wf-1",
    executorHint: null,
    onCancel: () => {},
    onUpdateWithoutAgent: () => {},
    onCreateWithoutAgent: () => {},
    onCreateWithPlanMode: () => {},
    ...overrides,
  };
}

describe("computeDisabledReason (start-task)", () => {
  it("returns null when nothing is missing", () => {
    expect(computeDisabledReason(makeProps(), KIND_START)).toBeNull();
  });

  it("returns null while a submission is in flight", () => {
    expect(
      computeDisabledReason(makeProps({ isCreatingTask: true, hasTitle: false }), KIND_START),
    ).toBeNull();
  });

  it("flags missing title first", () => {
    expect(
      computeDisabledReason(
        makeProps({ hasTitle: false, hasRepositorySelection: false }),
        KIND_START,
      ),
    ).toBe(REASON_TITLE);
  });

  it("flags missing repository selection", () => {
    expect(
      computeDisabledReason(makeProps({ hasRepositorySelection: false }), KIND_START),
    ).toBe(REASON_REPO);
  });

  it("flags missing branch", () => {
    expect(computeDisabledReason(makeProps({ branch: "" }), KIND_START)).toBe(REASON_BRANCH);
  });

  it("flags missing workspace in create mode", () => {
    expect(computeDisabledReason(makeProps({ workspaceId: null }), KIND_START)).toBe(
      REASON_WORKSPACE,
    );
  });

  it("flags missing workflow in create mode", () => {
    expect(computeDisabledReason(makeProps({ effectiveWorkflowId: null }), KIND_START)).toBe(
      REASON_WORKFLOW,
    );
  });

  it("ignores missing workspace/workflow outside create mode", () => {
    expect(
      computeDisabledReason(
        makeProps({ isCreateMode: false, isEditMode: true, workspaceId: null }),
        KIND_START,
      ),
    ).toBeNull();
  });

  it("flags missing agent profile for start-task button", () => {
    expect(computeDisabledReason(makeProps({ agentProfileId: "" }), KIND_START)).toBe(
      REASON_AGENT,
    );
  });
});

describe("computeDisabledReason (update)", () => {
  it("only flags missing title for the update button", () => {
    expect(
      computeDisabledReason(
        makeProps({ hasTitle: false, hasRepositorySelection: false, agentProfileId: "" }),
        KIND_UPDATE,
      ),
    ).toBe(REASON_TITLE);
  });

  it("returns null for update when title is present, even with other gaps", () => {
    expect(
      computeDisabledReason(
        makeProps({ hasRepositorySelection: false, agentProfileId: "" }),
        KIND_UPDATE,
      ),
    ).toBeNull();
  });
});

describe("computeDisabledReason (default)", () => {
  it("does not require agent outside session mode", () => {
    expect(computeDisabledReason(makeProps({ agentProfileId: "" }), KIND_DEFAULT)).toBeNull();
  });

  it("requires agent in session mode", () => {
    expect(
      computeDisabledReason(
        makeProps({ isSessionMode: true, agentProfileId: "" }),
        KIND_DEFAULT,
      ),
    ).toBe(REASON_AGENT);
  });
});
