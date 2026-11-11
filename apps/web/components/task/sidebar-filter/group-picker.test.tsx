import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { GroupPicker, useSidebarGroupLabel } from "./group-picker";

const PLUGIN = "sidebar-picker-test";
const FACET = "facet:sidebar-picker-test:tags";
const GROUP_SELECT = "group-key-select";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
});

describe("sidebar Group picker", () => {
  it("appends plugin facets after builtins and emits the persisted namespaced option", () => {
    pluginRegistry
      .forPlugin(PLUGIN)
      .registerTaskListFacet({ id: "tags", label: "Tag", getValues: () => [] });
    const onChange = vi.fn();
    render(<GroupPicker value="repository" onChange={onChange} />);
    fireEvent.click(screen.getByTestId(GROUP_SELECT));
    expect(
      screen.getAllByRole("option").map((option) => option.getAttribute("aria-label")),
    ).toEqual(["None", "Repository", "Workflow", "Workflow step", "Executor type", "State", "Tag"]);
    fireEvent.click(screen.getByRole("option", { name: "Tag" }));
    expect(onChange).toHaveBeenCalledWith(FACET);
  });

  it.each(["legacy-group", "facet:invalid", ""])(
    "keeps an unusual saved group %j editable without overwriting it",
    (savedGroup) => {
      const value = savedGroup as GroupKey;
      const onChange = vi.fn();
      render(<GroupPicker value={value} onChange={onChange} />);
      const summary = renderHook(() => useSidebarGroupLabel(value));

      expect(summary.result.current).toBe("None");
      expect(screen.getByTestId(GROUP_SELECT).textContent).toContain("None");
      expect(onChange).not.toHaveBeenCalled();

      fireEvent.click(screen.getByTestId(GROUP_SELECT));
      fireEvent.click(screen.getByRole("option", { name: "Repository" }));
      expect(onChange).toHaveBeenCalledExactlyOnceWith("repository");
    },
  );

  it("shows the builtin fallback without writing and restores a saved facet when it registers", () => {
    const onChange = vi.fn();
    render(<GroupPicker value={FACET} onChange={onChange} />);
    const label = renderHook(() => useSidebarGroupLabel(FACET));
    expect(label.result.current).toBe("None");
    expect(screen.getByTestId(GROUP_SELECT).textContent).toContain("None");
    act(() => {
      pluginRegistry
        .forPlugin(PLUGIN)
        .registerTaskListFacet({ id: "tags", label: "Tag", getValues: () => [] });
    });
    expect(label.result.current).toBe("Tag");
    expect(screen.getByTestId(GROUP_SELECT).textContent).toContain("Tag");
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    expect(label.result.current).toBe("None");
    expect(onChange).not.toHaveBeenCalled();
  });
});
