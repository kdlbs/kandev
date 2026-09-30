import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  buildChangesHistoryTimelineRows,
  changesCommitExpansionKey,
  type ChangesHistoryTimelineRow,
} from "./changes-timeline-model";
import type { CommitItem } from "./commit-row";
import { ChangesTimelineViewport } from "./changes-timeline-viewport";

type TestRow = { key: string; index: number };
type GroupedTestRow = { key: string; group: string; kind: string; repository?: string };
type DeepGroupedTestRow = { key: string; section: string; repository: string; commit: string };
const TIMELINE_ROW_SELECTOR = "[data-changes-timeline-row]";

const resizeObservers: ControlledResizeObserver[] = [];

class ControlledResizeObserver {
  private readonly targets = new Set<Element>();

  constructor(private readonly callback: ResizeObserverCallback) {
    resizeObservers.push(this);
  }

  observe(target: Element) {
    this.targets.add(target);
  }

  unobserve(target: Element) {
    this.targets.delete(target);
  }

  disconnect() {
    this.targets.clear();
  }

  emit() {
    const entries = [...this.targets].map((target) => ({
      target,
      contentRect: { width: 800, height: 600 },
      borderBoxSize: [{ inlineSize: 800, blockSize: 600 }],
    })) as unknown as ResizeObserverEntry[];
    this.callback(entries, this as unknown as ResizeObserver);
  }
}

function TestViewport({ rows }: { rows: TestRow[] }) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  return (
    <div
      ref={setScrollElement}
      data-testid="scroll-viewport"
      style={{ height: 600, overflow: "auto" }}
    >
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={scrollElement}
        estimateSize={() => 28}
        renderRow={(row) => <div data-testid={`timeline-row-${row.index}`}>{row.index}</div>}
      />
    </div>
  );
}

function HistoricalTestViewport({ rows }: { rows: ChangesHistoryTimelineRow[] }) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  return (
    <div
      ref={setScrollElement}
      data-testid="history-scroll-viewport"
      style={{ height: 600, overflow: "auto" }}
    >
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={scrollElement}
        estimateSize={() => 28}
        renderRow={(row, index) => (
          <div data-testid={`history-row-${index}`} data-history-kind={row.kind}>
            {row.key}
          </div>
        )}
      />
    </div>
  );
}

function makeLargeHistoryRows(kind: "pr" | "commits"): ChangesHistoryTimelineRow[] {
  if (kind === "pr") {
    const files = Array.from({ length: 50_000 }, (_, index) => ({
      path: `src/provider-file-${String(index).padStart(5, "0")}.ts`,
      prKey: "github:workspace/repository#42",
      status: "modified" as const,
      repository_name: "backend",
    }));
    return buildChangesHistoryTimelineRows(
      [
        {
          kind: "pr",
          sectionKey: "current-pr",
          label: "Pull request files",
          testId: "pr-files-section",
          collapsed: false,
          files,
          collapsedRepositories: new Set(),
        },
      ],
      new Set(),
    );
  }

  const commits = Array.from({ length: 50_000 }, (_, index) => {
    const sha = `sha-${String(index).padStart(5, "0")}`;
    const commit: CommitItem = {
      commit_sha: sha,
      commit_message: "Historical change",
      insertions: 1,
      deletions: 0,
      statsAvailable: true,
      detailTarget: { source: "local", sha, repo: "backend" },
      repository_name: "backend",
    };
    return { commit, detail: { expanded: false, status: "idle" as const, files: [] } };
  });
  const collapsedCommitKeys = new Set(
    commits.map(({ commit }) => changesCommitExpansionKey("local", commit.detailTarget)),
  );
  return buildChangesHistoryTimelineRows(
    [
      {
        kind: "commits",
        sectionKey: "local",
        label: "Commits",
        testId: "commits-section",
        collapsed: false,
        commits,
        collapsedRepositories: new Set(),
        collapsedDirectories: new Set(),
        layout: "flat",
      },
    ],
    collapsedCommitKeys,
  );
}

beforeEach(() => {
  resizeObservers.length = 0;
  vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
});

afterEach(() => {
  cleanup();
  resizeObservers.length = 0;
  vi.unstubAllGlobals();
});

it("preserves three nested group owners for virtual commit detail rows", async () => {
  const rows: DeepGroupedTestRow[] = [
    { key: "commit", section: "commits", repository: "repo-a", commit: "target-a" },
    { key: "file", section: "commits", repository: "repo-a", commit: "target-a" },
  ];
  function DeepGroupedViewport() {
    const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
    return (
      <div ref={setScrollElement} style={{ height: 600, overflow: "auto" }}>
        <ChangesTimelineViewport
          rows={rows}
          scrollElement={scrollElement}
          estimateSize={() => 28}
          getGroups={(row) => [
            { key: row.section, testId: "history-section-group" },
            { key: row.repository, testId: "history-repository-group" },
            { key: row.commit, attributes: { id: "inline-target" } },
          ]}
          renderRow={(row) => <div data-testid={`deep-${row.key}`}>{row.key}</div>}
        />
      </div>
    );
  }
  render(<DeepGroupedViewport />);

  act(() => resizeObservers.forEach((observer) => observer.emit()));
  await waitFor(() => expect(screen.getByTestId("deep-file")).toBeTruthy());
  const section = screen.getByTestId("history-section-group");
  const repository = screen.getByTestId("history-repository-group");
  const commit = document.getElementById("inline-target");
  expect(section.contains(repository)).toBe(true);
  expect(repository.contains(commit)).toBe(true);
  expect(commit?.contains(screen.getByTestId("deep-file"))).toBe(true);
});

it("keeps virtual rows inside their section and repository groups", async () => {
  const rows: GroupedTestRow[] = [
    { key: "section", group: "unstaged", kind: "section" },
    { key: "repository", group: "unstaged", repository: "repo-a", kind: "repository" },
    { key: "file", group: "unstaged", repository: "repo-a", kind: "file" },
  ];
  function GroupedViewport() {
    const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
    return (
      <div ref={setScrollElement} style={{ height: 600, overflow: "auto" }}>
        <ChangesTimelineViewport
          rows={rows}
          scrollElement={scrollElement}
          estimateSize={() => 28}
          getGroup={(row) => ({ key: row.group, testId: "section-group" })}
          getNestedGroup={(row) =>
            row.repository
              ? {
                  key: row.repository,
                  testId: "repository-group",
                  attributes: { "data-repository-name": row.repository },
                }
              : undefined
          }
          renderRow={(row) => <div data-testid={`grouped-${row.kind}`}>{row.kind}</div>}
        />
      </div>
    );
  }
  const view = render(<GroupedViewport />);

  act(() => resizeObservers.forEach((observer) => observer.emit()));
  await waitFor(() => expect(screen.getByTestId("grouped-file")).toBeTruthy());
  expect(screen.getByTestId("section-group").contains(screen.getByTestId("grouped-section"))).toBe(
    true,
  );
  expect(screen.getByTestId("repository-group").contains(screen.getByTestId("grouped-file"))).toBe(
    true,
  );
  expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR)).toHaveLength(3);
});

it("bounds 50,000 working rows and keeps the final row reachable", async () => {
  const rows = Array.from({ length: 50_000 }, (_, index) => ({
    key: `row-${index}`,
    index,
  }));
  const view = render(<TestViewport rows={rows} />);
  const scrollViewport = screen.getByTestId("scroll-viewport");

  act(() => resizeObservers.forEach((observer) => observer.emit()));
  await waitFor(() => expect(screen.getByTestId("timeline-row-0")).toBeTruthy());
  expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);

  Object.defineProperty(scrollViewport, "scrollHeight", {
    configurable: true,
    value: 1_400_000,
  });
  Object.defineProperty(scrollViewport, "clientHeight", { configurable: true, value: 600 });
  act(() => {
    scrollViewport.scrollTop = 1_399_400;
    scrollViewport.dispatchEvent(new Event("scroll"));
  });
  expect(scrollViewport.scrollTop).toBe(1_399_400);

  await waitFor(() => expect(screen.getByTestId("timeline-row-49999")).toBeTruthy());
  expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);
});

it.each(["pr", "commits"] as const)(
  "bounds 50,000 %s rows and keeps the final history row reachable",
  async (kind) => {
    const rows = makeLargeHistoryRows(kind);
    const finalRow = rows.at(-1);
    if (!finalRow) throw new Error("Expected a final history row");
    const view = render(<HistoricalTestViewport rows={rows} />);
    const scrollViewport = screen.getByTestId("history-scroll-viewport");

    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId("history-row-0")).toBeTruthy());
    expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);

    Object.defineProperty(scrollViewport, "scrollHeight", {
      configurable: true,
      value: 1_500_000,
    });
    Object.defineProperty(scrollViewport, "clientHeight", { configurable: true, value: 600 });
    act(() => {
      scrollViewport.scrollTop = 1_499_400;
      scrollViewport.dispatchEvent(new Event("scroll"));
    });

    const finalRowIndex = rows.length - 1;
    await waitFor(() => expect(screen.getByTestId(`history-row-${finalRowIndex}`)).toBeTruthy());
    expect(screen.getByTestId(`history-row-${finalRowIndex}`).textContent).toContain(finalRow.key);
    expect(view.container.querySelectorAll(TIMELINE_ROW_SELECTOR).length).toBeLessThanOrEqual(120);
  },
);
