import { describe, expect, it } from "vitest";
import {
  autoFixRoundForState,
  clampAutoFixRound,
  findCIAutomationStateForPR,
  normalizeAutoFixMaxRounds,
} from "./ci-automation";
import type { TaskCIPRAutomationState, TaskPR } from "@/lib/types/github";

function makePR(repositoryID: string, prNumber = 42): TaskPR {
  return {
    repository_id: repositoryID,
    pr_number: prNumber,
  } as TaskPR;
}

function makeState(
  repositoryID: string,
  prNumber: number,
  roundCount: number,
  exhaustedAt: string | null = null,
): TaskCIPRAutomationState {
  return {
    repository_id: repositoryID,
    pr_number: prNumber,
    auto_fix_round_count: roundCount,
    auto_fix_exhausted_at: exhaustedAt,
  } as TaskCIPRAutomationState;
}

describe("CI automation helpers", () => {
  it("finds PR automation state by repository id and PR number", () => {
    const states = [makeState("", 42, 1), makeState("repo-1", 42, 2), makeState("repo-1", 7, 3)];

    expect(findCIAutomationStateForPR(states, makePR("repo-1", 42))).toBe(states[1]);
    expect(findCIAutomationStateForPR(states, makePR("", 42))).toBe(states[0]);
    expect(findCIAutomationStateForPR(states, makePR("repo-missing", 42))).toBeUndefined();
  });

  it("returns an empty round state when no PR state exists", () => {
    expect(autoFixRoundForState(undefined, 12)).toEqual({
      current: 0,
      max: 12,
      exhausted: false,
    });
  });

  it("treats only backend exhaustion timestamps as paused", () => {
    expect(autoFixRoundForState(makeState("repo-1", 42, 10), 10)).toEqual({
      current: 10,
      max: 10,
      exhausted: false,
    });
    expect(autoFixRoundForState(makeState("repo-1", 42, 3, "2026-06-18T11:00:00Z"), 10)).toEqual({
      current: 3,
      max: 10,
      exhausted: true,
    });
  });

  it("normalizes max rounds and clamps current rounds", () => {
    expect(normalizeAutoFixMaxRounds(undefined)).toBe(10);
    expect(normalizeAutoFixMaxRounds(null)).toBe(10);
    expect(normalizeAutoFixMaxRounds(Number.NaN)).toBe(10);
    expect(normalizeAutoFixMaxRounds(0)).toBe(1);
    expect(normalizeAutoFixMaxRounds(-4)).toBe(1);
    expect(normalizeAutoFixMaxRounds(12.8)).toBe(12);

    expect(clampAutoFixRound(undefined, 10)).toBe(0);
    expect(clampAutoFixRound(Number.NaN, 10)).toBe(0);
    expect(clampAutoFixRound(-2, 10)).toBe(0);
    expect(clampAutoFixRound(4.9, 10)).toBe(4);
    expect(clampAutoFixRound(14, 10)).toBe(10);
  });
});
