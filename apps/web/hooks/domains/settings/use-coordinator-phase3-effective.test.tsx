import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useCoordinatorPhase3Effective } from "./use-coordinator-phase3-effective";

const flags = vi.hoisted(() => ({ current: {} as Record<string, boolean> }));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => flags.current[name] ?? false,
}));

describe("useCoordinatorPhase3Effective", () => {
  it("is true only when coordinator, phase 2 and phase 3 are all on", () => {
    for (const coordinator of [false, true]) {
      for (const coordinatorPhase2 of [false, true]) {
        for (const coordinatorPhase3 of [false, true]) {
          flags.current = { coordinator, coordinatorPhase2, coordinatorPhase3 };
          const { result } = renderHook(() => useCoordinatorPhase3Effective());
          expect(result.current).toBe(coordinator && coordinatorPhase2 && coordinatorPhase3);
        }
      }
    }
  });
});
