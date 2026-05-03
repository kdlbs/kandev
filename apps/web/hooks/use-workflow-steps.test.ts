import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const listWorkflowStepsMock = vi.fn();

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: (...args: unknown[]) => listWorkflowStepsMock(...args),
}));

import { useWorkflowSteps, stepPlaceholder } from "./use-workflow-steps";

beforeEach(() => {
  listWorkflowStepsMock.mockReset();
});

describe("useWorkflowSteps", () => {
  it("returns empty steps and skips fetching when workflowId is empty", async () => {
    const { result } = renderHook(() => useWorkflowSteps(""));
    expect(result.current.steps).toEqual([]);
    expect(result.current.loading).toBe(false);
    // Give any (unwanted) fetch a chance to fire.
    await new Promise((r) => setTimeout(r, 10));
    expect(listWorkflowStepsMock).not.toHaveBeenCalled();
  });

  it("fetches and exposes sorted steps for a non-empty workflowId", async () => {
    listWorkflowStepsMock.mockResolvedValueOnce({
      steps: [
        { id: "s2", name: "Second", position: 2 },
        { id: "s1", name: "First", position: 1 },
      ],
    });
    const { result } = renderHook(() => useWorkflowSteps("wf-1"));
    // loading starts true on initial render with a truthy workflowId so the
    // dropdown can show "Loading steps…" before the fetch lands.
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.steps).toEqual([
      { id: "s1", name: "First" },
      { id: "s2", name: "Second" },
    ]);
  });

  it("clears stale steps synchronously when workflowId changes", async () => {
    listWorkflowStepsMock.mockResolvedValueOnce({
      steps: [{ id: "s1", name: "First", position: 1 }],
    });
    const { result, rerender } = renderHook(({ id }: { id: string }) => useWorkflowSteps(id), {
      initialProps: { id: "wf-1" },
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.steps).toHaveLength(1);

    // Make the second fetch hang so we can observe the in-flight transition.
    let resolveSecond: (value: { steps: { id: string; name: string; position: number }[] }) => void;
    listWorkflowStepsMock.mockReturnValueOnce(
      new Promise((res) => {
        resolveSecond = res;
      }),
    );
    rerender({ id: "wf-2" });

    // Steps cleared synchronously; loading flips true while the fetch is in flight.
    expect(result.current.steps).toEqual([]);
    expect(result.current.loading).toBe(true);

    resolveSecond!({ steps: [{ id: "s9", name: "Ninth", position: 9 }] });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.steps).toEqual([{ id: "s9", name: "Ninth" }]);
  });

  it("clears steps and stops loading when the fetch rejects", async () => {
    listWorkflowStepsMock.mockRejectedValueOnce(new Error("network down"));
    const { result } = renderHook(() => useWorkflowSteps("wf-err"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.steps).toEqual([]);
  });
});

describe("stepPlaceholder", () => {
  it("prompts to select a workflow first when workflowId is empty", () => {
    expect(stepPlaceholder("", false, 5)).toBe("Select a workflow first");
  });

  it('reports "Loading steps…" while the fetch is in flight', () => {
    expect(stepPlaceholder("wf-1", true, 0)).toBe("Loading steps…");
  });

  it("reports the empty-workflow case when there are no steps", () => {
    expect(stepPlaceholder("wf-1", false, 0)).toBe("No steps in this workflow");
  });

  it('reports "Select step" once steps are available', () => {
    expect(stepPlaceholder("wf-1", false, 3)).toBe("Select step");
  });
});
