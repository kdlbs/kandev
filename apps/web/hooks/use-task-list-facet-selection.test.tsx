import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useTaskListFacetSelection } from "./use-task-list-facet-selection";

const FACET_KEY = "facet:plugin:tags";

const baseProps = {
  facetKeys: [FACET_KEY],
  coreSort: "updated" as never,
  coreGroup: "none" as never,
  tasks: [{ id: "task-1" }] as never[],
  facetValues: {},
  onCoreSortChange: vi.fn(),
  onCoreGroupChange: vi.fn(),
};

describe("useTaskListFacetSelection", () => {
  it("returns to core controls when the selected facet unregisters", () => {
    const { result, rerender } = renderHook((props) => useTaskListFacetSelection(props), {
      initialProps: baseProps,
    });

    act(() => result.current.selectSort(FACET_KEY));
    act(() => result.current.selectGroup(FACET_KEY));
    expect(result.current.sort).toBe(FACET_KEY);
    expect(result.current.group).toBe(FACET_KEY);

    rerender({ ...baseProps, facetKeys: [] });

    expect(result.current.sort).toBe("updated");
    expect(result.current.group).toBe("none");
  });
});
