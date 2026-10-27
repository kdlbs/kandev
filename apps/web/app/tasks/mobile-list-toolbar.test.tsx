import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MobileListToolbar } from "./mobile-list-toolbar";

const store = {
  workflows: { items: [] as Array<{ id: string; workspaceId: string; name: string }> },
  repositories: { itemsByWorkspaceId: {} as Record<string, Array<{ id: string; name: string }>> },
};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof store) => unknown) => selector(store),
}));

afterEach(() => {
  cleanup();
  store.workflows.items = [];
  store.repositories.itemsByWorkspaceId = {};
});

const defaultProps = {
  open: true,
  searchQuery: "",
  onSearchChange: vi.fn(),
  isSearchLoading: false,
  resultFilters: [],
  filterDraft: [],
  hasLinkScope: false,
  onClearLinkScope: vi.fn(),
  onRemoveResultFilter: vi.fn(),
  onBeginFilterEdit: vi.fn(),
  onSetFilterDraft: vi.fn(),
  onCancelFilterEdit: vi.fn(),
  onApplyFilterEdit: vi.fn(),
  onClearFilterDraft: vi.fn(),
  workspaceId: "ws-a",
};

describe("MobileListToolbar", () => {
  it("keeps the search row hidden until the header opens it", () => {
    const { rerender } = render(<MobileListToolbar {...defaultProps} open={false} />);
    expect(screen.queryByTestId("mobile-list-search-row")).toBeNull();

    rerender(<MobileListToolbar {...defaultProps} />);
    expect(screen.getByTestId("mobile-list-search-row")).toBeTruthy();
    expect(screen.getByTestId("mobile-list-search-input")).toBe(document.activeElement);
  });

  it("shows temporary summaries and a separate clear action for explicit links", () => {
    store.workflows.items = [{ id: "wf-a", workspaceId: "ws-a", name: "Workflow A" }];
    const onClearLinkScope = vi.fn();
    const onRemoveResultFilter = vi.fn();
    render(
      <MobileListToolbar
        {...defaultProps}
        resultFilters={[
          { id: "temporary-state", dimension: "state", op: "is", value: "review" },
          { id: "temporary-workflow", dimension: "workflow", op: "is", value: "wf-a" },
        ]}
        hasLinkScope
        onClearLinkScope={onClearLinkScope}
        onRemoveResultFilter={onRemoveResultFilter}
      />,
    );

    expect(screen.getByTestId("mobile-list-temporary-filter-summary").textContent).toContain(
      "State: Review",
    );
    expect(screen.getByTestId("mobile-list-temporary-filter-summary").textContent).toContain(
      "Workflow: Workflow A",
    );
    expect(screen.getByTestId("mobile-list-link-scope").textContent).toContain(
      "Linked result scope",
    );
    fireEvent.click(screen.getByTestId("mobile-list-remove-filter-state"));
    expect(onRemoveResultFilter).toHaveBeenCalledWith("state");
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(onClearLinkScope).toHaveBeenCalledOnce();
  });

  it("opens the focused filter sheet from the one toolbar button", () => {
    render(
      <MobileListToolbar
        {...defaultProps}
        resultFilters={[{ id: "temporary-state", dimension: "state", op: "is", value: "review" }]}
      />,
    );

    fireEvent.click(screen.getByTestId("mobile-list-result-filters"));
    expect(screen.getByTestId("mobile-list-result-filters-drawer")).toBeTruthy();
    expect(screen.getByTestId("mobile-list-result-filters").textContent).toContain("Filters (1)");
  });
});
