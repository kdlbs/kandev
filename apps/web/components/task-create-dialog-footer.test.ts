import { describe, it, expect } from "vitest";
import {
  computeDisabledReason,
  REASON_TITLE,
  REASON_REPO,
  REASON_BRANCH,
  REASON_WORKSPACE,
  REASON_WORKFLOW,
  REASON_AGENT,
  REASON_DESCRIPTION,
} from "./task-create-dialog-footer";
import type { ButtonKind, TaskCreateDialogFooterProps } from "./task-create-dialog-footer";

const KIND_START: ButtonKind = "start-task";
const KIND_UPDATE: ButtonKind = "update";
const KIND_DEFAULT: ButtonKind = "default";

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
    hasAllBranches: true,
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
    expect(computeDisabledReason(makeProps({ hasRepositorySelection: false }), KIND_START)).toBe(
      REASON_REPO,
    );
  });

  it("flags missing branch", () => {
    expect(computeDisabledReason(makeProps({ hasAllBranches: false }), KIND_START)).toBe(
      REASON_BRANCH,
    );
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
    expect(computeDisabledReason(makeProps({ agentProfileId: "" }), KIND_START)).toBe(REASON_AGENT);
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
      computeDisabledReason(makeProps({ isSessionMode: true, agentProfileId: "" }), KIND_DEFAULT),
    ).toBe(REASON_AGENT);
  });

  it("flags missing session description in session mode", () => {
    expect(
      computeDisabledReason(
        makeProps({ isSessionMode: true, hasDescription: false }),
        KIND_DEFAULT,
      ),
    ).toBe(REASON_DESCRIPTION);
  });

  it("does not flag missing description for passthrough profiles", () => {
    expect(
      computeDisabledReason(
        makeProps({ isSessionMode: true, hasDescription: false, isPassthroughProfile: true }),
        KIND_DEFAULT,
      ),
    ).toBeNull();
  });

  it("ignores base reasons in session mode to match DefaultSubmitButton disabled logic", () => {
    // In session mode the default button is only disabled by !agentProfileId or
    // missing description — NOT by missing title/repo/branch/workspace/workflow.
    // The tooltip must not contradict that state.
    expect(
      computeDisabledReason(
        makeProps({
          isSessionMode: true,
          hasTitle: false,
          hasRepositorySelection: false,
          hasAllBranches: false,
        }),
        KIND_DEFAULT,
      ),
    ).toBeNull();
  });
});

describe("computeDisabledReason (submitBlockedReason)", () => {
  const REASON = "Preparing kandev repository…";

  it("returns the supplied reason for start-task even when nothing is missing", () => {
    expect(computeDisabledReason(makeProps({ submitBlockedReason: REASON }), KIND_START)).toBe(
      REASON,
    );
  });

  it("returns the supplied reason for default in create mode", () => {
    expect(computeDisabledReason(makeProps({ submitBlockedReason: REASON }), KIND_DEFAULT)).toBe(
      REASON,
    );
  });

  it("returns the supplied reason for update mode (overrides title check)", () => {
    expect(
      computeDisabledReason(
        makeProps({ submitBlockedReason: REASON, hasTitle: false }),
        KIND_UPDATE,
      ),
    ).toBe(REASON);
  });

  it("still suppresses the reason while a submission is in flight", () => {
    expect(
      computeDisabledReason(
        makeProps({ submitBlockedReason: REASON, isCreatingTask: true }),
        KIND_START,
      ),
    ).toBeNull();
  });

  it("ignores empty/null reason and falls back to normal logic", () => {
    expect(computeDisabledReason(makeProps({ submitBlockedReason: null }), KIND_START)).toBeNull();
    expect(computeDisabledReason(makeProps({ submitBlockedReason: "" }), KIND_START)).toBeNull();
  });
});
