import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ListToolbar } from "./list-toolbar";

afterEach(() => cleanup());

function renderToolbar() {
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
        repoFilter=""
        onRepoFilterChange={vi.fn()}
        repoOptions={["acme/api", "kdlbs/kandev"]}
        onRefresh={vi.fn()}
      />
    </TooltipProvider>,
  );
}

describe("ListToolbar", () => {
  it("opens the repository filter with a search input", async () => {
    renderToolbar();

    fireEvent.click(screen.getByText("All repos"));

    expect(await screen.findByPlaceholderText("Filter repositories...")).toBeTruthy();
    expect(screen.getByText("kdlbs/kandev")).toBeTruthy();
  });
});
