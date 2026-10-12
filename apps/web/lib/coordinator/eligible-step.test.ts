import { describe, expect, it } from "vitest";
import type { EligibleStepNode } from "./eligible-step";
import { eligibleStep } from "./eligible-step";

// Ports apps/backend/internal/coordinator/eligibility_test.go's table so the
// client-side convenience filter (system-design/proposal-cards.md#cards
// "Edit form options") matches the approve route's authoritative check.

function node(overrides: Partial<EligibleStepNode> & { id: string }): EligibleStepNode {
  return {
    isStart: false,
    allowManualMove: false,
    autoStartOnEnter: false,
    pullFromStepId: null,
    ...overrides,
  };
}

describe("eligibleStep", () => {
  it("returns false for an unknown step id", () => {
    const steps = [node({ id: "start", isStart: true })];
    expect(eligibleStep(steps, "missing")).toBe(false);
  });

  it("is eligible for the plain start step", () => {
    const steps = [node({ id: "start", isStart: true })];
    expect(eligibleStep(steps, "start")).toBe(true);
  });

  it("is eligible for a non-start step that allows manual moves", () => {
    const steps = [
      node({ id: "start", isStart: true }),
      node({ id: "middle", allowManualMove: true }),
    ];
    expect(eligibleStep(steps, "middle")).toBe(true);
  });

  it("is ineligible for a step that is neither the start step nor manual-move-enabled", () => {
    const steps = [node({ id: "start", isStart: true }), node({ id: "locked" })];
    expect(eligibleStep(steps, "locked")).toBe(false);
  });

  it("is ineligible for a step that itself auto-starts an agent on enter", () => {
    const steps = [node({ id: "start", isStart: true, autoStartOnEnter: true })];
    expect(eligibleStep(steps, "start")).toBe(false);
  });

  it("is ineligible for a direct pull_from_step_id feeder of an auto-start step", () => {
    const steps = [
      node({ id: "start", isStart: true }),
      node({ id: "auto", allowManualMove: true, autoStartOnEnter: true, pullFromStepId: "start" }),
    ];
    expect(eligibleStep(steps, "start")).toBe(false);
  });

  it("is ineligible for a transitive feeder of an auto-start step through a chain", () => {
    const steps = [
      node({ id: "start", isStart: true }),
      node({ id: "mid", allowManualMove: true, pullFromStepId: "start" }),
      node({ id: "auto", allowManualMove: true, autoStartOnEnter: true, pullFromStepId: "mid" }),
    ];
    expect(eligibleStep(steps, "start")).toBe(false);
    expect(eligibleStep(steps, "mid")).toBe(false);
  });

  it("terminates on a cyclic feeder graph with no auto-start step (eligible)", () => {
    const steps = [
      node({ id: "a", isStart: true, allowManualMove: true, pullFromStepId: "b" }),
      node({ id: "b", allowManualMove: true, pullFromStepId: "a" }),
    ];
    expect(eligibleStep(steps, "a")).toBe(true);
  });

  it("does not let an unrelated auto-start step affect other steps", () => {
    const steps = [
      node({ id: "start", isStart: true }),
      node({ id: "other", allowManualMove: true }),
      node({ id: "auto", autoStartOnEnter: true, pullFromStepId: "other" }),
    ];
    expect(eligibleStep(steps, "start")).toBe(true);
  });
});
