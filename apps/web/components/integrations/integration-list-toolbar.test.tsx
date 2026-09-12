import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationListToolbar } from "./integration-list-toolbar";
import i18n from "i18next";

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
});

function renderToolbar(customQuery = "open", count = 3, loading = false) {
  const onCommitCustomQuery = vi.fn();
  const onRefresh = vi.fn();
  render(
    <IntegrationListToolbar
      title="Pull requests"
      count={count}
      loading={loading}
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
  it.each([
    ["en", 1, "Result 1"],
    ["pt-pt", 1, "Resultado 1"],
    ["pt-pt", 3, "Resultados 3"],
    ["zh-cn", 3, "共 3 项结果"],
  ])("localizes the whole result count in %s for %s", async (locale, count, expected) => {
    await i18n.changeLanguage(locale);
    renderToolbar("open", count);
    const result = screen.getByTestId("integration-mobile-result-count");
    expect(result.textContent).toBe(expected);
    expect(result.querySelector(".tabular-nums")?.textContent).toBe(String(count));
  });

  it("uses separate localized loading copy without a stale count", () => {
    renderToolbar("open", 3, true);
    expect(screen.getByTestId("integration-mobile-result-count").textContent).toBe(
      "Loading results…",
    );
  });

  it("renders shared title, count, filter, query, and responsive refresh controls", () => {
    const { onRefresh } = renderToolbar();
    expect(screen.getByTestId("change-toolbar-title").textContent).toBe("Pull requests");
    expect(screen.getAllByText("3")).toHaveLength(2);
    expect(screen.getByTestId("integration-mobile-result-count").textContent).toMatch(
      /Results\s+3/,
    );
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
