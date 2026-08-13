import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationListToolbar } from "./integration-list-toolbar";

afterEach(cleanup);

function renderToolbar(customQuery = "open") {
  const onCommitCustomQuery = vi.fn();
  const onRefresh = vi.fn();
  render(
    <IntegrationListToolbar
      title="Pull requests"
      count={3}
      loading={false}
      lastFetchedAt={null}
      customQuery={customQuery}
      committedQuery="open"
      onCustomQueryChange={vi.fn()}
      onCommitCustomQuery={onCommitCustomQuery}
      onRefresh={onRefresh}
      filter={<button type="button">All repositories</button>}
      queryPlaceholder="Search pull requests"
      titleTestId="change-toolbar-title"
      queryTestId="change-toolbar-query"
      refreshTestId="change-toolbar-refresh"
    />,
  );
  return { onCommitCustomQuery, onRefresh };
}

describe("IntegrationListToolbar", () => {
  it("renders shared title, count, filter, query, and responsive refresh controls", () => {
    const { onRefresh } = renderToolbar();
    expect(screen.getByTestId("change-toolbar-title").textContent).toBe("Pull requests");
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByRole("button", { name: "All repositories" })).toBeTruthy();
    expect(screen.getByTestId("change-toolbar-query").getAttribute("placeholder")).toBe(
      "Search pull requests",
    );
    const refreshButtons = screen.getAllByTestId("change-toolbar-refresh");
    expect(refreshButtons).toHaveLength(2);
    fireEvent.click(refreshButtons[0]);
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("commits a dirty query on Enter or blur", () => {
    const { onCommitCustomQuery } = renderToolbar("draft");
    const query = screen.getByTestId("change-toolbar-query");
    fireEvent.keyDown(query, { key: "Enter" });
    fireEvent.blur(query);
    expect(onCommitCustomQuery).toHaveBeenCalledTimes(2);
  });
});
