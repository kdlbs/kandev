import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ListToolbar } from "./list-toolbar";

afterEach(() => cleanup());

const ALL_REPOS_LABEL = "All repos";
const REPO_SEARCH_PLACEHOLDER = "Filter repositories...";
const REPO_DROPDOWN_TEST_ID = "github-repo-filter-dropdown";
const LEGACY_REPO_DROPDOWN_TEST_ID = "github-repo-filter-search";
const KANDEV_REPO = "kdlbs/kandev";

function renderToolbar({ repoFilter = "" }: { repoFilter?: string } = {}) {
  const onRepoFilterChange = vi.fn();
  render(
    <TooltipProvider>
      <ListToolbar
        title="Review requested"
        count={2}
        loading={false}
        lastFetchedAt={null}
        customQuery="is:open"
        committedQuery="is:open"
        onCustomQueryChange={vi.fn()}
        onCommitCustomQuery={vi.fn()}
        repoFilter={repoFilter}
        onRepoFilterChange={onRepoFilterChange}
        repoOptions={["acme/api", KANDEV_REPO]}
        onRefresh={vi.fn()}
      />
    </TooltipProvider>,
  );
  return { onRepoFilterChange };
}

describe("ListToolbar", () => {
  it("opens the repository filter with a search input", async () => {
    renderToolbar();

    fireEvent.click(screen.getByText(ALL_REPOS_LABEL));

    expect(await screen.findByPlaceholderText(REPO_SEARCH_PLACEHOLDER)).toBeTruthy();
    expect(screen.getByTestId(REPO_DROPDOWN_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(REPO_DROPDOWN_TEST_ID).dataset.legacyTestid).toBe(
      LEGACY_REPO_DROPDOWN_TEST_ID,
    );
    expect(screen.getByText(KANDEV_REPO)).toBeTruthy();
  });

  it("filters and selects a repository", async () => {
    const { onRepoFilterChange } = renderToolbar();

    fireEvent.click(screen.getByText(ALL_REPOS_LABEL));
    fireEvent.change(await screen.findByPlaceholderText(REPO_SEARCH_PLACEHOLDER), {
      target: { value: "kdlbs" },
    });
    fireEvent.click(await screen.findByRole("option", { name: KANDEV_REPO }));

    expect(onRepoFilterChange).toHaveBeenCalledWith(KANDEV_REPO);
  });

  it("clears the repository filter from the All repos option", async () => {
    const { onRepoFilterChange } = renderToolbar({ repoFilter: KANDEV_REPO });

    fireEvent.click(screen.getByText(KANDEV_REPO));
    fireEvent.click(await screen.findByRole("option", { name: ALL_REPOS_LABEL }));

    expect(onRepoFilterChange).toHaveBeenCalledWith("");
  });

  it("preserves the repository filter when the selected repository is reselected", async () => {
    const { onRepoFilterChange } = renderToolbar({ repoFilter: KANDEV_REPO });

    fireEvent.click(screen.getByText(KANDEV_REPO));
    fireEvent.click(await screen.findByRole("option", { name: KANDEV_REPO }));

    expect(onRepoFilterChange).not.toHaveBeenCalled();
  });
});
