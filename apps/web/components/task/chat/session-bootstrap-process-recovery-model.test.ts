import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type { TaskStatusSummaryActiveError } from "@/lib/types/task-status-summary";
import { buildRecoveryCardModel } from "./session-bootstrap-recovery-model";

const translate = ((key: string) => key) as unknown as TFunction;

describe("process exit recovery card model", () => {
  it("localizes a recognized mise process exit with status and prerequisite", () => {
    const model = buildRecoveryCardModel({
      error: error({
        operation: "start",
        code: "process_exited",
        reason: "mise_config_untrusted",
        exit_code: 1,
        detail: "The agent exited before startup completed.",
      }),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate,
    });
    expect(model).toMatchObject({
      titleKey: "task:sessionBootstrapProcessMiseTitle",
      summary: "task:sessionBootstrapProcessMiseBody",
      prerequisite: "task:sessionBootstrapProcessMisePrerequisite",
      hasTypedProcessCause: true,
      primaryCause: { code: "process_exited", reason: "mise_config_untrusted", exit_code: 1 },
    });
    expect(model.detailFields).toContainEqual({
      label: "task:sessionErrorDetailExitCode",
      value: "1",
    });
    expect(JSON.stringify(model)).not.toContain("/private/workspace");
  });

  it("keeps the mise trust command as an interpolation value", () => {
    const translateCommand = vi.fn((key: string, options?: { command?: string }) =>
      options?.command ? `${key}:${options.command}` : key,
    ) as unknown as TFunction;
    const model = buildRecoveryCardModel({
      error: error({
        operation: "start",
        code: "process_exited",
        reason: "mise_config_untrusted",
        exit_code: 1,
      }),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate: translateCommand,
    });
    expect(model.prerequisite).toBe("task:sessionBootstrapProcessMisePrerequisite:mise trust");
  });

  it("keeps an unknown ordinary exit unattributed while showing its status", () => {
    const model = buildRecoveryCardModel({
      error: error({
        operation: "resume",
        code: "process_exited",
        reason: "unclassified",
        exit_code: 3,
      }),
      manualFailure: null,
      manualError: null,
      recoveryNotice: null,
      translate,
    });
    expect(model).toMatchObject({
      titleKey: "task:sessionBootstrapProcessExitedTitle",
      summary: "task:sessionBootstrapProcessExitedBody",
      prerequisite: null,
      hasTypedProcessCause: true,
    });
    expect(model.detailFields).toContainEqual({
      label: "task:sessionErrorDetailExitCode",
      value: "3",
    });
  });
});

function error(
  cause: NonNullable<TaskStatusSummaryActiveError["causes"]>[number],
): TaskStatusSummaryActiveError {
  return {
    session_id: "session-1",
    stamp: "failure-1",
    occurred_at: "2026-11-07T11:00:00Z",
    preview: "The agent could not start.",
    phase: "bootstrap",
    causes: [cause],
  };
}
