import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";
import { buildRecoveryCardModel } from "./session-bootstrap-recovery-model";

const translate = ((key: string) => key) as unknown as TFunction;

function error(
  causes: NonNullable<TaskStatusSummaryActiveError["causes"]>,
  stamp = "failure-1",
): TaskStatusSummaryActiveError {
  return {
    session_id: "session-1",
    stamp,
    occurred_at: "2026-09-30T11:00:00Z",
    preview: "The agent could not start.",
    phase: "bootstrap",
    causes,
  };
}

describe("buildRecoveryCardModel", () => {
  it("makes typed model evidence the primary cause without opening details", () => {
    const translateCause = vi.fn((key: string) => key) as unknown as TFunction;
    const model = buildRecoveryCardModel({
      error: error([
        {
          operation: "start",
          code: "model_unavailable",
          reason: "requested_not_advertised",
          requested_model: "claude-opus-4-8",
          prompt_not_sent: true,
        } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
      ]),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate: translateCause,
      agentDisplayName: "Auggie",
    });

    expect(model).toMatchObject({
      titleKey: "task:sessionBootstrapModelUnavailableTitle",
      summary: "task:sessionBootstrapModelUnavailableBody",
      showSummary: true,
      primaryCause: {
        operation: "start",
        code: "model_unavailable",
        requested_model: "claude-opus-4-8",
        prompt_not_sent: true,
      },
      noPromptSent: true,
      freshStartWarning: true,
    });
    expect(translateCause).toHaveBeenCalledWith("task:sessionBootstrapModelUnavailableBody", {
      agent: "Auggie",
      model: "claude-opus-4-8",
    });
  });

  it("keeps a cause visible beside an independent read-only workspace outcome", () => {
    const model = buildRecoveryCardModel({
      error: error([
        {
          operation: "resume",
          code: "permission_mode_mismatch",
          reason: "effective_mismatch",
          requested_mode: "default",
          effective_mode: "plan",
        } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
      ]),
      automaticRecovery: {
        resumptionState: "resumed",
        error: null,
        notice: "workspace restored",
        recoveryFailure: { outcome: "workspace_read_only", resumeError: "resume failed" },
        resumeSession: async () => true,
      },
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate,
      agentDisplayName: "Auggie",
    });

    expect(model).toMatchObject({
      isReadOnly: true,
      titleKey: "task:sessionBootstrapModeMismatchTitle",
      summary: "task:sessionBootstrapModeMismatchBody",
      workspaceStatus: "task:sessionRecoveryWorkspaceReadOnly",
    });
  });

  it("retains different operations and selector identities with the same code", () => {
    const model = buildRecoveryCardModel({
      error: error([
        {
          operation: "resume",
          code: "model_selection_failed",
          reason: "application_failed",
          requested_model: "model-a",
          detail: "selection failed",
        } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
        {
          operation: "restore_workspace",
          code: "model_selection_failed",
          reason: "application_failed",
          requested_model: "model-b",
          detail: "selection failed",
        } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
      ]),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate,
    });

    expect(model.causes).toHaveLength(2);
    expect(model.primaryCause?.requested_model).toBe("model-a");
  });
});

it("uses the model actually attempted when its application failed", () => {
  const translateAttempt = ((key: string, options?: { model?: string }) =>
    `${key}:${options?.model ?? ""}`) as unknown as TFunction;
  const model = buildRecoveryCardModel({
    error: error([
      {
        operation: "start",
        code: "model_selection_failed",
        reason: "application_failed",
        requested_model: "primary",
        attempted_model: "alternate",
      } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
    ]),
    manualFailure: null,
    manualError: null,
    recoveryNotice: null,
    translate: translateAttempt,
    agentDisplayName: "Auggie",
  });

  expect(model.summary).toBe("task:sessionBootstrapModelSelectionFailedBody:alternate");
});

it("does not claim the configured model when the attempted selector is unsafe", () => {
  const model = buildRecoveryCardModel({
    error: error([
      {
        operation: "start",
        code: "model_selection_failed",
        reason: "application_failed",
        requested_model: "primary",
        attempted_model: "token=unsafe-selector",
      } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
    ]),
    manualFailure: null,
    manualError: null,
    recoveryNotice: null,
    translate,
    agentDisplayName: "Auggie",
  });

  expect(model.summary).toBe("task:sessionBootstrapModelSelectionFailedBodyUnknown");
});

describe("buildRecoveryCardModel request-local failures", () => {
  it("ignores a request-local failure owned by an earlier error stamp", () => {
    const model = buildRecoveryCardModel({
      error: error(
        [
          {
            operation: "resume",
            code: "model_unavailable",
            reason: "requested_not_advertised",
            requested_model: "current-model",
          } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
        ],
        "failure-current",
      ),
      manualFailure: {
        operation: "resume",
        sessionId: "session-1",
        errorStamp: "failure-previous",
        requestKey: "task-1\u0000session-1\u0000failure-previous",
        operationId: 2,
      },
      manualError: new Error("earlier request failed"),
      recoveryNotice: null,
      translate,
    });

    expect(model.causes).toHaveLength(1);
    expect(model.primaryCause?.requested_model).toBe("current-model");
    expect(model.summary).toBe("task:sessionBootstrapModelUnavailableBody");
  });

  it("uses localized cause-only copy when an unsafe selector is omitted", () => {
    const model = buildRecoveryCardModel({
      error: error([
        {
          operation: "start",
          code: "model_unavailable",
          reason: "requested_not_advertised",
          requested_model: "/home/private/model",
        } as NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
      ]),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate,
    });

    expect(model.titleKey).toBe("task:sessionBootstrapModelUnavailableTitle");
    expect(model.summary).toBe("task:sessionBootstrapModelUnavailableBodyUnknown");
    expect(JSON.stringify(model)).not.toContain("private");
  });
});
