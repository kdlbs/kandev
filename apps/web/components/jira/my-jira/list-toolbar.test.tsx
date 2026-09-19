import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DEFAULT_FILTERS } from "./filter-model";
import type { SavedView } from "./use-saved-views";
import { ListToolbar } from "./list-toolbar";

const DELETE_VIEW_TITLE = "Delete view";

const responsive = vi.hoisted(() => ({ isFinePointer: false, isMobile: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

const BUILTIN: SavedView = {
  id: "builtin:assigned",
  name: "",
  nameKey: "jira:builtinViewAssignedToMe",
  builtin: true,
  filters: DEFAULT_FILTERS,
};
const CUSTOM: SavedView = {
  id: "view-sprint-bugs",
  name: "Sprint bugs",
  filters: DEFAULT_FILTERS,
};

function renderToolbar(onDeleteView = vi.fn()) {
  return {
    onDeleteView,
    ...render(
      <ListToolbar
        searchText=""
        onSearchChange={vi.fn()}
        views={[BUILTIN, CUSTOM]}
        activeViewId={CUSTOM.id}
        onSelectView={vi.fn()}
        onDeleteView={onDeleteView}
        onSaveView={vi.fn()}
        count={1}
        loading={false}
        sort="updated"
        onSortChange={vi.fn()}
        onRefresh={vi.fn()}
        showJqlEditor={false}
        onToggleJqlEditor={vi.fn()}
      />,
    ),
  };
}

describe("Jira ListToolbar saved views", () => {
  afterEach(() => {
    cleanup();
    responsive.isMobile = false;
  });

  it("hands a phone saved view to a named sheet outside the closed picker", async () => {
    responsive.isMobile = true;
    const { onDeleteView } = renderToolbar();
    const trigger = screen.getByRole("button", { name: "Sprint bugs" });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    const sheet = await screen.findByRole("dialog", { name: "Delete Sprint bugs?" });
    expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
    expect(document.querySelector('[data-slot="popover-content"]')).toBeNull();
    fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(onDeleteView).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    fireEvent.click(await screen.findByRole("button", { name: "Delete Sprint bugs" }));
    await waitFor(() => expect(onDeleteView).toHaveBeenCalledExactlyOnceWith(CUSTOM.id));
  });

  it("keeps built-ins protected and confirms custom deletion inline on coarse pointers", async () => {
    const { onDeleteView } = renderToolbar();
    fireEvent.click(screen.getByRole("button", { name: "Sprint bugs" }));

    expect(screen.getAllByTitle(DELETE_VIEW_TITLE)).toHaveLength(1);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));

    expect(onDeleteView).not.toHaveBeenCalled();
    const confirmation = screen.getByRole("group", { name: "Delete Sprint bugs?" });
    expect(within(confirmation).getByRole("button", { name: "Cancel" }).className).toContain(
      "h-11",
    );
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(onDeleteView).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    fireEvent.click(screen.getByRole("button", { name: "Delete Sprint bugs" }));

    await waitFor(() => expect(onDeleteView).toHaveBeenCalledWith(CUSTOM.id));
    expect(onDeleteView).toHaveBeenCalledOnce();
  });
});
