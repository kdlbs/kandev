import { describe, expect, it } from "vitest";
import {
  autoFixRoundForState,
  clampAutoFixRound,
  findMRAutomationStateForMR,
  normalizeAutoFixMaxRounds,
} from "./mr-automation";
import type { TaskMR, TaskMRLifecycleState } from "@/lib/types/gitlab";

function makeMR(repositoryID: string, projectPath: string, mrIID = 42): TaskMR {
  return {
    repository_id: repositoryID,
    project_path: projectPath,
    mr_iid: mrIID,
  } as TaskMR;
}

function makeState(
  repositoryID: string,
  projectPath: string,
  mrIID: number,
  roundCount: number,
  exhaustedAt: string | null = null,
): TaskMRLifecycleState {
  return {
    repository_id: repositoryID,
    project_path: projectPath,
    mr_iid: mrIID,
    auto_fix_round_count: roundCount,
    auto_fix_exhausted_at: exhaustedAt,
  } as TaskMRLifecycleState;
}

describe("MR automation helpers", () => {
  it("finds MR automation state by repository id, project path, and MR iid", () => {
    const states = [
      makeState("", "group/a", 42, 1),
      makeState("repo-1", "group/a", 42, 2),
      makeState("repo-1", "group/a", 7, 3),
    ];

    expect(findMRAutomationStateForMR(states, makeMR("repo-1", "group/a", 42))).toBe(states[1]);
    expect(findMRAutomationStateForMR(states, makeMR("", "group/a", 42))).toBe(states[0]);
    expect(
      findMRAutomationStateForMR(states, makeMR("repo-missing", "group/a", 42)),
    ).toBeUndefined();
  });

  it("returns an empty round state when no MR state exists", () => {
    expect(autoFixRoundForState(undefined, 12)).toEqual({
      current: 0,
      max: 12,
      exhausted: false,
    });
  });

  it("treats only backend exhaustion timestamps as paused", () => {
    expect(autoFixRoundForState(makeState("repo-1", "group/a", 42, 10), 10)).toEqual({
      current: 10,
      max: 10,
      exhausted: false,
    });
    expect(
      autoFixRoundForState(makeState("repo-1", "group/a", 42, 3, "2026-06-18T11:00:00Z"), 10),
    ).toEqual({
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
