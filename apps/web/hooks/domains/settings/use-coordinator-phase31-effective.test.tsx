import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useCoordinatorPhase31Effective } from "./use-coordinator-phase31-effective";

const flags = vi.hoisted(() => ({ current: {} as Record<string, boolean> }));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => flags.current[name] ?? false,
}));

describe("useCoordinatorPhase31Effective", () => {
  it("is true only when coordinator, phases 2, 3 and 3.1 are all on", () => {
    const names = ["coordinator", "coordinatorPhase2", "coordinatorPhase3", "coordinatorPhase31"];
    for (let mask = 0; mask < 16; mask++) {
      flags.current = Object.fromEntries(names.map((n, i) => [n, Boolean(mask & (1 << i))]));
      const { result } = renderHook(() => useCoordinatorPhase31Effective());
      expect(result.current).toBe(mask === 15);
    }
  });
});
