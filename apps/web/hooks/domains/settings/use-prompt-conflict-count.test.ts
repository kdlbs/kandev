import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { CustomPrompt } from "@/lib/types/http";

const state = vi.hoisted(() => ({ prompts: [] as Partial<CustomPrompt>[] }));
vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ prompts: state.prompts, loaded: true, loading: false }),
}));

import { usePromptConflictCount } from "./use-prompt-conflict-count";

describe("usePromptConflictCount", () => {
  it("counts only overrides with an unresolved conflict", () => {
    state.prompts = [
      { name: "a", source: "override", conflict: true },
      { name: "b", source: "override" },
      { name: "c", source: "manual", conflict: true },
      { name: "d", source: "override", conflict: true },
    ];
    const { result } = renderHook(() => usePromptConflictCount());
    expect(result.current).toBe(2);
  });
});
