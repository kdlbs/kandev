import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { ViewHeaderRow } from "./view-manager";

const NEW_VIEW: SidebarView = {
  id: "view-new",
  name: "New view",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "repository",
  collapsedGroups: [],
};
const RENAME_INPUT_TEST_ID = "view-rename-input";

function headerProps(activeView: SidebarView | undefined = NEW_VIEW) {
  return {
    activeView,
    hasDraft: false,
    canDelete: true,
    onSaveOverwrite: vi.fn(),
    onSaveAs: vi.fn(),
    onRename: vi.fn(),
    onDiscard: vi.fn(),
    onDelete: vi.fn(),
    renameRequestedViewId: null as string | null,
    onRenameRequestHandled: vi.fn(),
  };
}

afterEach(cleanup);

describe("ViewHeaderRow external rename", () => {
  it("focuses and selects the requested active view name", async () => {
    const props = headerProps();
    props.renameRequestedViewId = NEW_VIEW.id;
    render(<ViewHeaderRow {...props} />);

    const input = await screen.findByTestId(RENAME_INPUT_TEST_ID);
    expect((input as HTMLInputElement).value).toBe("New view");
    expect(screen.getByRole("textbox", { name: "View name" })).toBe(input);
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).selectionStart).toBe(0);
    expect((input as HTMLInputElement).selectionEnd).toBe("New view".length);
    expect(props.onRenameRequestHandled).toHaveBeenCalledWith(NEW_VIEW.id);
  });

  it("keeps the created view when rename is cancelled", async () => {
    const props = headerProps();
    props.renameRequestedViewId = NEW_VIEW.id;
    render(<ViewHeaderRow {...props} />);

    await screen.findByTestId(RENAME_INPUT_TEST_ID);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByTestId(RENAME_INPUT_TEST_ID)).toBeNull();
    expect(screen.getByTestId("sidebar-filter-active-view-name").textContent).toBe("New view");
    expect(props.onRename).not.toHaveBeenCalled();
    expect(props.onDelete).not.toHaveBeenCalled();
  });

  it("exits stale rename mode if optimistic creation rolls back", async () => {
    const props = headerProps();
    props.renameRequestedViewId = NEW_VIEW.id;
    const { rerender } = render(<ViewHeaderRow {...props} />);
    await screen.findByTestId(RENAME_INPUT_TEST_ID);

    const restored = { ...headerProps({ ...NEW_VIEW, id: "view-all", name: "All tasks" }) };
    rerender(<ViewHeaderRow {...restored} />);

    await waitFor(() => {
      expect(screen.queryByTestId(RENAME_INPUT_TEST_ID)).toBeNull();
    });
    expect(screen.getByTestId("sidebar-filter-active-view-name").textContent).toBe("All tasks");
  });

  it("submits the requested rename without closing the surrounding popover", async () => {
    const props = headerProps();
    props.renameRequestedViewId = NEW_VIEW.id;
    render(<ViewHeaderRow {...props} />);
    const input = await screen.findByTestId(RENAME_INPUT_TEST_ID);

    fireEvent.change(input, { target: { value: "Focused work" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(props.onRename).toHaveBeenCalledWith(NEW_VIEW.id, "Focused work");
    expect(screen.queryByTestId(RENAME_INPUT_TEST_ID)).toBeNull();
  });
});
