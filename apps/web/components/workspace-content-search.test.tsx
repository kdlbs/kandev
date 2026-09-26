import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

vi.mock("@kandev/ui/command", () => ({
  CommandEmpty: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandGroup: ({
    children,
    heading,
    forceMount: _forceMount,
    ...props
  }: {
    children: ReactNode;
    heading: ReactNode;
    forceMount?: boolean;
  }) => (
    <section {...props}>
      <h2>{heading}</h2>
      {children}
    </section>
  ),
  CommandItem: ({
    children,
    onSelect,
    forceMount: _forceMount,
    value: _value,
    ...props
  }: {
    children: ReactNode;
    onSelect: () => void;
    forceMount?: boolean;
    value?: string;
  }) => (
    <button {...props} onClick={onSelect}>
      {children}
    </button>
  ),
}));

vi.mock("@/hooks/domains/session/use-repo-display-name", () => ({
  useRepoDisplayName: () => (repositoryName: string) =>
    repositoryName ? `Display ${repositoryName}` : undefined,
}));

vi.mock("@/components/ui/file-icon", () => ({
  FileIcon: ({ fileName }: { fileName: string }) => <span aria-label={`file ${fileName}`} />,
}));

import {
  WorkspaceContentSearch,
  getContentSearchResultValue,
  splitPreviewByMatches,
} from "./workspace-content-search";

const REPO_A = "repo-a";
const REPO_B = "repo-b";
const SHARED_PATH = "src/shared.ts";
const REPOSITORY_ATTRIBUTE = "data-repository";

const results: WorkspaceContentSearchResult[] = [
  {
    repository_name: REPO_A,
    path: SHARED_PATH,
    line: 12,
    column: 4,
    preview: "const needle = true",
    match_ranges: [{ start: 6, end: 12 }],
  },
  {
    repository_name: REPO_B,
    path: SHARED_PATH,
    line: 8,
    column: 2,
    preview: "return needle",
    match_ranges: [{ start: 7, end: 13 }],
  },
];

afterEach(() => cleanup());

describe("content search result identity", () => {
  it("keeps same-path results in different repositories distinct", () => {
    expect(getContentSearchResultValue(results[0])).not.toBe(
      getContentSearchResultValue(results[1]),
    );
  });
});

describe("splitPreviewByMatches", () => {
  it("uses backend UTF-16 offsets without splitting an emoji surrogate pair", () => {
    expect(splitPreviewByMatches("😀 needle", [{ start: 3, end: 9 }])).toEqual([
      { text: "😀 ", matched: false },
      { text: "needle", matched: true },
    ]);
  });

  it("clamps, sorts, and merges unsafe ranges", () => {
    expect(
      splitPreviewByMatches("abcdef", [
        { start: 4, end: 99 },
        { start: -2, end: 2 },
        { start: 1, end: 3 },
        { start: 3, end: 3 },
      ]),
    ).toEqual([
      { text: "abc", matched: true },
      { text: "d", matched: false },
      { text: "ef", matched: true },
    ]);
  });
});

describe("WorkspaceContentSearch", () => {
  it("groups results by raw repository while showing display labels", () => {
    const onSelect = vi.fn();
    render(
      <WorkspaceContentSearch
        results={results}
        isSearching={false}
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={onSelect}
      />,
    );

    const groups = screen.getAllByTestId("content-search-repo-group");
    expect(groups).toHaveLength(2);
    expect(groups[0].getAttribute(REPOSITORY_ATTRIBUTE)).toBe(REPO_A);
    expect(groups[1].getAttribute(REPOSITORY_ATTRIBUTE)).toBe(REPO_B);
    expect(within(groups[0]).getByText(`Display ${REPO_A}`)).toBeTruthy();

    const resultRows = screen.getAllByTestId("content-search-result");
    expect(resultRows[0].getAttribute("data-path")).toBe(SHARED_PATH);
    expect(resultRows[0].getAttribute(REPOSITORY_ATTRIBUTE)).toBe(REPO_A);
    expect(resultRows[0].getAttribute("data-line")).toBe("12");
    expect(within(resultRows[0]).getByText("12").classList.contains("tabular-nums")).toBe(true);
    expect(within(resultRows[0]).getByTestId("content-search-match").textContent).toBe("needle");

    fireEvent.click(resultRows[1]);
    expect(onSelect).toHaveBeenCalledWith(results[1]);
  });
});

describe("WorkspaceContentSearch in-progress state", () => {
  it("keeps the searching state visible next to partial results", () => {
    // Results arrive incrementally, so a populated list can still be growing.
    render(
      <WorkspaceContentSearch
        results={results}
        isSearching={true}
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getAllByTestId("content-search-result").length).toBeGreaterThan(0);
    const indicator = screen.getByTestId("content-search-in-progress");
    expect(indicator).toBeTruthy();

    // CommandList scrolls (max-h-72 overflow-y-auto), so existing in the DOM is
    // not enough: appended below the groups it drops out of view the moment the
    // early matches fill the palette. It has to lead the list and stick.
    const groups = screen.getAllByTestId("content-search-repo-group");
    expect(indicator.compareDocumentPosition(groups[0])).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(indicator.className).toContain("sticky");
    expect(indicator.className).toContain("top-0");
  });

  it("drops the searching state once the search finishes", () => {
    render(
      <WorkspaceContentSearch
        results={results}
        isSearching={false}
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );

    expect(screen.queryByTestId("content-search-in-progress")).toBeNull();
  });
});

describe("WorkspaceContentSearch labels", () => {
  it("uses a non-empty label for a single repository with an empty transport key", () => {
    render(
      <WorkspaceContentSearch
        results={[{ ...results[0], repository_name: "" }]}
        isSearching={false}
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByTestId("content-search-repo-group").getAttribute(REPOSITORY_ATTRIBUTE)).toBe(
      "",
    );
    expect(screen.getByText("Results")).toBeTruthy();
  });

  it("shows distinct empty, searching, and no-results states", () => {
    const { rerender } = render(
      <WorkspaceContentSearch
        results={[]}
        isSearching={false}
        error={null}
        search=""
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText("Type to search task contents…")).toBeTruthy();

    rerender(
      <WorkspaceContentSearch
        results={[]}
        isSearching
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText("Searching task workspace…")).toBeTruthy();

    rerender(
      <WorkspaceContentSearch
        results={[]}
        isSearching={false}
        error={null}
        search="needle"
        sessionId="session-1"
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByText("No content matches found.")).toBeTruthy();
  });

  it.each([
    ["session-unavailable", "Content search needs an active task session."],
    ["query-too-long", "Search queries are limited to 200 characters."],
    ["transport-error", "Search failed. Edit the query or reopen search to retry."],
  ] as const)("shows the %s state instead of no matches", (error, message) => {
    render(
      <WorkspaceContentSearch
        results={[]}
        isSearching={false}
        error={error}
        search="needle"
        sessionId={error === "session-unavailable" ? null : "session-1"}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByText(message)).toBeTruthy();
    expect(screen.queryByText("No content matches found.")).toBeNull();
  });
});
