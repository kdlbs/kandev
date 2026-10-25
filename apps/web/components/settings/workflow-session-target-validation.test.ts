import { describe, expect, it } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import {
  getWorkflowSessionTargetIssue,
  hasInvalidWorkflowSessionTargets,
} from "./workflow-session-target-validation";

function step(id: string, position: number, overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id,
    workflow_id: "workflow-1" as WorkflowStep["workflow_id"],
    name: id,
    position,
    color: "bg-blue-500",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

describe("workflow session target validation", () => {
  it("accepts an earlier direct-profile source", () => {
    const source = step("source", 0, { agent_profile_id: "profile-a" });
    const destination = step("destination", 1, {
      session_target: { kind: "step", step_id: source.id },
    });

    expect(getWorkflowSessionTargetIssue(destination, [source, destination])).toBeUndefined();
  });

  it.each([
    {
      name: "missing source",
      destination: step("destination", 1, {
        session_target: { kind: "step", step_id: "gone" },
      }),
      steps: [],
      kind: "missing-source",
    },
    {
      name: "source after destination",
      destination: step("destination", 1, {
        session_target: { kind: "step", step_id: "source" },
      }),
      steps: [step("source", 2, { agent_profile_id: "profile-a" })],
      kind: "not-earlier",
    },
    {
      name: "source without profile",
      destination: step("destination", 1, {
        session_target: { kind: "step", step_id: "source" },
      }),
      steps: [step("source", 0)],
      kind: "missing-profile",
    },
    {
      name: "indirect source",
      destination: step("destination", 1, {
        session_target: { kind: "step", step_id: "source" },
      }),
      steps: [
        step("source", 0, {
          agent_profile_id: "profile-a",
          session_target: { kind: "initial" },
        }),
      ],
      kind: "indirect-source",
    },
  ])("reports $name", ({ destination, steps, kind }) => {
    expect(getWorkflowSessionTargetIssue(destination, [destination, ...steps])).toMatchObject({
      kind,
    });
  });

  it("detects any invalid target before a workflow save", () => {
    const source = step("source", 0);
    const destination = step("destination", 1, {
      session_target: { kind: "step", step_id: source.id },
    });

    expect(hasInvalidWorkflowSessionTargets([source, destination])).toBe(true);
  });
});
