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

const virtualizerTestHooks = vi.hoisted(() => ({ measure: vi.fn() }));

vi.mock("@tanstack/react-virtual", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-virtual")>();
  return {
    ...actual,
    useVirtualizer: ((...args: Parameters<typeof actual.useVirtualizer>) => {
      const virtualizer = actual.useVirtualizer(...args);
      const measure = virtualizer.measure.bind(virtualizer);
      virtualizer.measure = () => {
        virtualizerTestHooks.measure();
        measure();
      };
      return virtualizer;
    }) as typeof actual.useVirtualizer,
  };
});

type TestRow = { key: string; index: number };
type GroupedTestRow = { key: string; group: string; kind: string; repository?: string };
type DeepGroupedTestRow = { key: string; section: string; repository: string; commit: string };
type FocusFallbackRow = { key: string; section: string; label: string };
const SCROLL_VIEWPORT_TEST_ID = "scroll-viewport";
const TIMELINE_ROW_SELECTOR = "[data-changes-timeline-row]";

const resizeObservers: ControlledResizeObserver[] = [];
const originalFontsProperty = Object.getOwnPropertyDescriptor(document, "fonts");

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

  emit(width = 800, height = 600) {
    const entries = [...this.targets].map((target) => ({
      target,
      contentRect: { width, height },
      borderBoxSize: [{ inlineSize: width, blockSize: height }],
    })) as unknown as ResizeObserverEntry[];
    this.callback(entries, this as unknown as ResizeObserver);
  }

  emitTarget(target: Element, width: number, height: number) {
    if (!this.targets.has(target)) return;
    const entry = {
      target,
      contentRect: { width, height },
      borderBoxSize: [{ inlineSize: width, blockSize: height }],
    } as unknown as ResizeObserverEntry;
    this.callback([entry], this as unknown as ResizeObserver);
  }
}

function TestViewport({ rows, estimate = 28 }: { rows: TestRow[]; estimate?: number }) {
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
        estimateSize={() => estimate}
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

function FocusFallbackViewport({ hideFocusedRow }: { hideFocusedRow: boolean }) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  const rows: FocusFallbackRow[] = [
    { key: "section-a", section: "section-a", label: "Section A" },
    ...(!hideFocusedRow
      ? [{ key: "focused-file-a", section: "section-a", label: "Focused file" }]
      : []),
    { key: "section-b", section: "section-b", label: "Section B" },
    { key: "file-b", section: "section-b", label: "Other section file" },
  ];
  return (
    <div ref={setScrollElement} style={{ height: 600, overflow: "auto" }}>
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={scrollElement}
        estimateSize={() => 28}
        getGroups={(row) => [{ key: row.section }]}
        renderRow={(row) => (
          <button type="button" data-changes-row-focus data-testid={row.key}>
            {row.label}
          </button>
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
  virtualizerTestHooks.measure.mockClear();
  vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
});

afterEach(() => {
  cleanup();
  resizeObservers.length = 0;
  if (originalFontsProperty) Object.defineProperty(document, "fonts", originalFontsProperty);
  else Reflect.deleteProperty(document, "fonts");
  vi.restoreAllMocks();
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

it("returns focus to the surviving owner section when the focused row is removed", async () => {
  const view = render(<FocusFallbackViewport hideFocusedRow={false} />);
  act(() => resizeObservers.forEach((observer) => observer.emit()));
  const focusedRow = await screen.findByTestId("focused-file-a");
  act(() => focusedRow.focus());
  expect(document.activeElement).toBe(focusedRow);

  view.rerender(<FocusFallbackViewport hideFocusedRow />);

  await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("section-a")));
  expect(document.activeElement).not.toBe(screen.getByTestId("section-b"));
});

it("remeasures rows when width, locale, font, or pointer mode changes and preserves the scroll anchor", async () => {
  const pointerMode = new EventTarget() as MediaQueryList;
  let coarsePointer = false;
  Object.defineProperties(pointerMode, {
    matches: { get: () => coarsePointer },
    media: { value: "(pointer: coarse)" },
  });
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => pointerMode),
  );
  const fontEvents = new EventTarget();
  Object.defineProperty(document, "fonts", { configurable: true, value: fontEvents });
  const measure = virtualizerTestHooks.measure;
  const rows = Array.from({ length: 200 }, (_, index) => ({ key: `row-${index}`, index }));
  render(<TestViewport rows={rows} />);
  const scrollViewport = screen.getByTestId(SCROLL_VIEWPORT_TEST_ID);
  act(() => resizeObservers.forEach((observer) => observer.emit()));
  await waitFor(() => expect(screen.getByTestId("timeline-row-0")).toBeTruthy());
  Object.defineProperty(scrollViewport, "scrollHeight", { configurable: true, value: 5_600 });
  Object.defineProperty(scrollViewport, "clientHeight", { configurable: true, value: 600 });
  act(() => {
    scrollViewport.scrollTop = 280;
    scrollViewport.dispatchEvent(new Event("scroll"));
  });
  await waitFor(() => expect(screen.getByTestId("timeline-row-10")).toBeTruthy());
  const anchorScrollTop = scrollViewport.scrollTop;
  measure.mockClear();

  const beforeWidthChange = measure.mock.calls.length;
  Object.defineProperty(scrollViewport, "clientWidth", { configurable: true, value: 640 });
  act(() => resizeObservers.forEach((observer) => observer.emitTarget(scrollViewport, 640, 600)));
  await waitFor(() => expect(measure.mock.calls.length).toBeGreaterThan(beforeWidthChange));
  expect(scrollViewport.scrollTop).toBe(anchorScrollTop);

  const beforePointerChange = measure.mock.calls.length;
  coarsePointer = true;
  act(() => pointerMode.dispatchEvent(new Event("change")));
  await waitFor(() => expect(measure.mock.calls.length).toBeGreaterThan(beforePointerChange));
  const beforeLanguageChange = measure.mock.calls.length;
  document.documentElement.lang = "pt-pt";
  await waitFor(() => expect(measure.mock.calls.length).toBeGreaterThan(beforeLanguageChange));
  const beforeFontsChange = measure.mock.calls.length;
  act(() => fontEvents.dispatchEvent(new Event("loadingdone")));
  await waitFor(() => expect(measure.mock.calls.length).toBeGreaterThan(beforeFontsChange));
  expect(scrollViewport.scrollTop).toBe(anchorScrollTop);
});

// @covers AC-UI-BOUNDED-CHANGES-001.6
it.each(["font", "locale", "width", "pointer"] as const)(
  "preserves measured row spacing after %s invalidation without new row observations",
  async (trigger) => {
    const fontEvents = new EventTarget();
    Object.defineProperty(document, "fonts", { configurable: true, value: fontEvents });
    const pointerMode = new EventTarget();
    let coarsePointer = false;
    Object.defineProperty(pointerMode, "matches", { get: () => coarsePointer });
    vi.stubGlobal("matchMedia", () => pointerMode);
    const heights = [24, 52, 24, 36, 24];
    const rows = heights.map((_, index) => ({ key: `measured-${index}`, index }));
    const view = render(<TestViewport rows={rows} estimate={34} />);
    act(() => {
      resizeObservers.forEach((observer) =>
        observer.emitTarget(screen.getByTestId(SCROLL_VIEWPORT_TEST_ID), 800, 600),
      );
    });
    const wrappers = Array.from(
      view.container.querySelectorAll<HTMLDivElement>(TIMELINE_ROW_SELECTOR),
    );
    wrappers.forEach((wrapper, index) => {
      Object.defineProperty(wrapper, "offsetHeight", { configurable: true, value: heights[index] });
    });
    act(() => {
      wrappers.forEach((wrapper, index) => {
        resizeObservers.forEach((observer) => observer.emitTarget(wrapper, 800, heights[index]));
      });
    });
    const positions = () => wrappers.map((wrapper) => wrapper.style.transform);
    const expected = [0, 24, 76, 100, 136].map((top) => `translateY(${top}px)`);
    await waitFor(() => expect(positions()).toEqual(expected));
    virtualizerTestHooks.measure.mockClear();
    const previousLanguage = document.documentElement.lang;
    try {
      act(() => {
        if (trigger === "font") fontEvents.dispatchEvent(new Event("loadingdone"));
        if (trigger === "locale") document.documentElement.lang = `${previousLanguage}-changed`;
        if (trigger === "width") {
          resizeObservers.forEach((observer) =>
            observer.emitTarget(screen.getByTestId(SCROLL_VIEWPORT_TEST_ID), 640, 600),
          );
        }
        if (trigger === "pointer") {
          coarsePointer = true;
          pointerMode.dispatchEvent(new Event("change"));
        }
      });
      await waitFor(() => {
        expect(virtualizerTestHooks.measure).toHaveBeenCalled();
        expect(positions()).toEqual(expected);
      });
    } finally {
      document.documentElement.lang = previousLanguage;
    }
  },
);

it("bounds 50,000 working rows and keeps the final row reachable", async () => {
  const rows = Array.from({ length: 50_000 }, (_, index) => ({
    key: `row-${index}`,
    index,
  }));
  const view = render(<TestViewport rows={rows} />);
  const scrollViewport = screen.getByTestId(SCROLL_VIEWPORT_TEST_ID);

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
