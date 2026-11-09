import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { StepType, TaskCreateWorkflowStepsResult } from "./task-create-dialog-types";
import { buildWorkflowAgentOverrideValidation } from "./task-create-dialog-workflow-agent-override-validation";

const profiles = [
  {
    id: "source",
    label: "Source",
    agent_id: "agent-source",
    agent_name: "source",
    cli_passthrough: false,
    enabled: true,
  },
  {
    id: "replacement",
    label: "Replacement",
    agent_id: "agent-replacement",
    agent_name: "replacement",
    cli_passthrough: false,
    enabled: true,
  },
] satisfies AgentProfileOption[];

const implementedSteps: StepType[] = [
  {
    id: "analysis",
    title: "Analysis",
    position: 0,
    is_start_step: true,
  },
  {
    id: "implement",
    title: "Implement",
    position: 1,
    agent_profile_id: "source",
  },
  {
    id: "pr",
    title: "PR",
    position: 2,
    session_target: { kind: "step", step_id: "implement" },
    prompt: "Ship {{task_prompt}}",
  },
];

function selected(
  status: TaskCreateWorkflowStepsResult["status"],
  steps: StepType[] = [],
): TaskCreateWorkflowStepsResult {
  return { status, steps, retry: () => undefined };
}

const base = {
  profiles,
  replacementOptions: profiles.map((profile) => ({
    value: profile.id,
    label: profile.label,
    renderLabel: () => profile.label,
  })),
  overrides: { source: "replacement" },
  isCreateMode: true,
};

describe("workflow agent override submit validation", () => {
  it("groups fixed profiles and step recipients from successful metadata", () => {
    const result = buildWorkflowAgentOverrideValidation({
      ...base,
      selectedWorkflowSteps: selected("success", implementedSteps),
    });

    expect(result.loading).toBe(false);
    expect(result.error).toBe(false);
    expect(result.invalid).toBe(false);
    expect(result.blockedReason).toBeUndefined();
    expect(result.rows).toEqual([
      expect.objectContaining({
        sourceProfileId: "source",
        stepIds: ["implement", "pr"],
        stepNames: ["Implement", "PR"],
        replacementProfileId: "replacement",
        replacementAvailable: true,
      }),
    ]);
  });

  it("treats loading and error metadata as unresolved selected replacements", () => {
    const loading = buildWorkflowAgentOverrideValidation({
      ...base,
      selectedWorkflowSteps: selected("loading"),
    });
    expect(loading.loading).toBe(true);
    expect(loading.error).toBe(false);
    expect(loading.blockedReason).toBeTruthy();

    const failed = buildWorkflowAgentOverrideValidation({
      ...base,
      selectedWorkflowSteps: selected("error"),
    });
    expect(failed.loading).toBe(false);
    expect(failed.error).toBe(true);
    expect(failed.rows).toEqual([]);
    expect(failed.blockedReason).toBeTruthy();
  });

  it("does not block ordinary creation while metadata is idle, loading, or failed", () => {
    const ordinary = { ...base, overrides: {} };
    for (const status of ["idle", "loading", "error"] as const) {
      const result = buildWorkflowAgentOverrideValidation({
        ...ordinary,
        selectedWorkflowSteps: selected(status),
      });
      expect(result.blockedReason).toBeUndefined();
    }
  });

  it("treats a successful empty step result as authoritative", () => {
    const result = buildWorkflowAgentOverrideValidation({
      ...base,
      selectedWorkflowSteps: selected("success"),
    });
    expect(result.loading).toBe(false);
    expect(result.error).toBe(false);
    expect(result.rows).toEqual([]);
    expect(result.blockedReason).toBeUndefined();
  });

  it("blocks an unavailable replacement in create mode and leaves edit mode unblocked", () => {
    const args = {
      ...base,
      selectedWorkflowSteps: selected("success", implementedSteps),
      replacementOptions: base.replacementOptions.filter(
        (option) => option.value !== "replacement",
      ),
    };
    expect(buildWorkflowAgentOverrideValidation(args).blockedReason).toBeTruthy();
    expect(
      buildWorkflowAgentOverrideValidation({ ...args, isCreateMode: false }).blockedReason,
    ).toBeUndefined();
  });
});
