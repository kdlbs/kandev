import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DEFAULT_FILTERS } from "./filter-model";
import type { SavedView } from "./use-saved-views";
import { ListToolbar } from "./list-toolbar";

const DELETE_VIEW_TITLE = "Delete view";
const CUSTOM_VIEW_NAME = "Sprint bugs";

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
  name: CUSTOM_VIEW_NAME,
  filters: DEFAULT_FILTERS,
};

function renderToolbar(
  onDeleteView = vi.fn(),
  onSetDefaultView = vi.fn(),
  options: { defaultMutationPending?: boolean; viewMutationPending?: boolean } = {},
) {
  const onSelectView = vi.fn();
  return {
    onDeleteView,
    onSetDefaultView,
    onSelectView,
    ...render(
      <ListToolbar
        searchText=""
        onSearchChange={vi.fn()}
        views={[BUILTIN, CUSTOM]}
        activeViewId={CUSTOM.id}
        defaultViewId={CUSTOM.id}
        viewsReady
        defaultMutationPending={options.defaultMutationPending ?? false}
        viewMutationPending={options.viewMutationPending ?? false}
        onSelectView={onSelectView}
        onSetDefaultView={onSetDefaultView}
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
    responsive.isFinePointer = false;
  });

  it("hands a phone saved view to a named sheet outside the closed picker", async () => {
    responsive.isMobile = true;
    const { onDeleteView } = renderToolbar();
    const trigger = screen.getByRole("button", { name: CUSTOM_VIEW_NAME });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    const sheet = await screen.findByRole("dialog", { name: `Delete ${CUSTOM_VIEW_NAME}?` });
    expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
    expect(document.querySelector('[data-slot="popover-content"]')).toBeNull();
    fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(onDeleteView).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    fireEvent.click(await screen.findByRole("button", { name: `Delete ${CUSTOM_VIEW_NAME}` }));
    await waitFor(() => expect(onDeleteView).toHaveBeenCalledExactlyOnceWith(CUSTOM.id));
  });

  it("keeps built-ins protected and confirms custom deletion inline on coarse pointers", async () => {
    const { onDeleteView } = renderToolbar();
    fireEvent.click(screen.getByRole("button", { name: CUSTOM_VIEW_NAME }));

    expect(screen.getAllByTitle(DELETE_VIEW_TITLE)).toHaveLength(1);
    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));

    expect(onDeleteView).not.toHaveBeenCalled();
    const confirmation = screen.getByRole("group", { name: `Delete ${CUSTOM_VIEW_NAME}?` });
    expect(within(confirmation).getByRole("button", { name: "Cancel" }).className).toContain(
      "h-11",
    );
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(onDeleteView).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTitle(DELETE_VIEW_TITLE));
    fireEvent.click(screen.getByRole("button", { name: `Delete ${CUSTOM_VIEW_NAME}` }));

    await waitFor(() => expect(onDeleteView).toHaveBeenCalledWith(CUSTOM.id));
    expect(onDeleteView).toHaveBeenCalledOnce();
  });

  it("sets and clears the default without selecting or deleting the view", async () => {
    const { onDeleteView, onSelectView, onSetDefaultView } = renderToolbar();
    fireEvent.click(screen.getByRole("button", { name: CUSTOM_VIEW_NAME }));

    const currentDefault = screen.getByRole("button", {
      name: `Clear ${CUSTOM_VIEW_NAME} as default view`,
    });
    expect(currentDefault.getAttribute("aria-pressed")).toBe("true");
    const makeBuiltinDefault = screen.getByRole("button", {
      name: "Set Assigned to me as default view",
    });
    expect(makeBuiltinDefault.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(makeBuiltinDefault);

    expect(onSetDefaultView).toHaveBeenCalledExactlyOnceWith(BUILTIN.id);
    expect(onSelectView).not.toHaveBeenCalled();
    expect(onDeleteView).not.toHaveBeenCalled();
  });

  it("keeps the default action touch-sized in the phone picker", () => {
    responsive.isMobile = true;
    renderToolbar();
    fireEvent.click(screen.getByRole("button", { name: CUSTOM_VIEW_NAME }));

    expect(
      screen.getByRole("button", { name: "Set Assigned to me as default view" }).className,
    ).toContain("h-12 w-12");
  });

  it("keeps the active default star visible on fine-pointer desktop", () => {
    responsive.isFinePointer = true;
    renderToolbar();
    fireEvent.click(screen.getByRole("button", { name: CUSTOM_VIEW_NAME }));

    const star = screen.getByRole("button", {
      name: `Clear ${CUSTOM_VIEW_NAME} as default view`,
    });
    expect(star.className).toContain("opacity-100");
    expect(star.className).not.toContain("opacity-0");
  });

  it("disables saving while a view deletion is pending", () => {
    renderToolbar(vi.fn(), vi.fn(), { defaultMutationPending: true });

    expect(
      (screen.getByTitle("Save current filters as a view") as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
