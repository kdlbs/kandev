import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { PromptSyncConflict } from "@/lib/types/prompt-sync";
import { usePromptConflictReview } from "./use-prompt-conflict-review";

const conflict = (name: string): PromptSyncConflict => ({
  name,
  path: `.kandev/prompts/${name}.md`,
  local_content: `Local ${name}`,
  repository_content: `Repo ${name}`,
});

describe("usePromptConflictReview", () => {
  it("starts inactive and defaults every opened conflict to keep local", () => {
    const { result } = renderHook(() => usePromptConflictReview());
    expect(result.current.active).toBe(false);
    act(() => result.current.open([conflict("a"), conflict("b")]));
    expect(result.current.active).toBe(true);
    expect(result.current.choices).toEqual({ a: "local", b: "local" });
  });

  it("records a choice per prompt and clears everything on close", () => {
    const { result } = renderHook(() => usePromptConflictReview());
    act(() => result.current.open([conflict("a"), conflict("b")]));
    act(() => result.current.setChoice("b", "repository"));
    expect(result.current.choices).toEqual({ a: "local", b: "repository" });
    act(() => result.current.close());
    expect(result.current.active).toBe(false);
    expect(result.current.conflicts).toEqual([]);
    expect(result.current.choices).toEqual({});
  });

  it("keeps earlier choices when reopened with more conflicts", () => {
    const { result } = renderHook(() => usePromptConflictReview());
    act(() => result.current.open([conflict("a")]));
    act(() => result.current.setChoice("a", "repository"));
    act(() => result.current.open([conflict("a"), conflict("c")]));
    expect(result.current.choices).toEqual({ a: "repository", c: "local" });
  });
});
