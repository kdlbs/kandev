import { describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useAuthorExclusionDraft } from "./use-author-exclusion-draft";
import type { AuthorExclusionStep } from "@/lib/types/author-exclusion";

const step: AuthorExclusionStep = {
  workflow_id: "wf-1",
  workflow_name: "Office Default",
  step_id: "step-review",
  step_name: "Review",
  role: "reviewer",
  enabled: false,
};

describe("useAuthorExclusionDraft", () => {
  it("reports only switches that differ from the saved value", () => {
    const { result } = renderHook(() => useAuthorExclusionDraft([step]));
    expect(result.current.changes).toEqual([]);

    act(() => result.current.setValue("step-review:reviewer", true));
    expect(result.current.changes).toEqual([
      { step_id: "step-review", role: "reviewer", enabled: true },
    ]);

    act(() => result.current.setValue("step-review:reviewer", false));
    expect(result.current.changes).toEqual([]);
  });

  it("clears the draft when the saved steps change", () => {
    const { result, rerender } = renderHook(({ steps }) => useAuthorExclusionDraft(steps), {
      initialProps: { steps: [step] },
    });
    act(() => result.current.setValue("step-review:reviewer", true));
    rerender({ steps: [{ ...step, enabled: true }] });
    expect(result.current.draft).toEqual({});
    expect(result.current.changes).toEqual([]);
  });
});
