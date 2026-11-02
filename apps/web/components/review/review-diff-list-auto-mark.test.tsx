import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { createRef, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReviewDiffListProps } from "./review-diff-list";
import { reviewFileKey, type ReviewFile } from "./types";

type ObserverRecord = {
  callback: IntersectionObserverCallback;
  instance: IntersectionObserver;
  options?: IntersectionObserverInit;
  targets: Element[];
  active: boolean;
};

const observerRecords: ObserverRecord[] = [];
const diffViewerProps: Array<{ enableWalkthroughAnnotations?: boolean; filePath: string }> = [];

vi.mock("@/components/diff", () => ({
  FileDiffViewer: (props: { enableWalkthroughAnnotations?: boolean; filePath: string }) => {
    diffViewerProps.push(props);
    return <div>{props.filePath}</div>;
  },
  DiffErrorBoundary: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => null,
  FileActionsMenuItems: () => null,
}));
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  ExternalVcsFileMenuItem: () => null,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: vi.fn(),
  updateFileContent: vi.fn(),
}));
vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["unified", vi.fn()],
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/hooks/domains/session/use-base-branch-by-repo", () => ({
  useBaseBranchByRepo: () => ({}),
}));
vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useActiveTaskPR: () => null,
}));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => null }));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { ReviewDiffList } from "./review-diff-list";

function file(path: string): ReviewFile {
  return {
    path,
    diff: "@@ -1 +1 @@\n-a\n+b\n",
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
  };
}

function installIntersectionObserver() {
  class MockIntersectionObserver implements IntersectionObserver {
    readonly root: Element | Document | null = null;
    readonly rootMargin = "0px";
    readonly thresholds: readonly number[] = [];
    private readonly record: ObserverRecord;

    constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
      this.record = { callback, instance: this, options, targets: [], active: true };
      observerRecords.push(this.record);
    }

    disconnect() {
      this.record.active = false;
    }
    observe(target: Element) {
      this.record.targets.push(target);
    }
    takeRecords(): IntersectionObserverEntry[] {
      return [];
    }
    unobserve() {}
  }

  vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
}

function firePosition(record: ObserverRecord, above: boolean, scrollTop: number) {
  const root = record.options?.root as HTMLElement;
  const changedScroll = root.scrollTop !== scrollTop;
  const top = above ? Math.min(50, 150 - scrollTop) : 150;
  Object.defineProperty(record.targets[0], "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top }),
  });
  root.scrollTop = scrollTop;
  Object.defineProperty(root, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top: 100 }),
  });
  if (changedScroll) fireEvent.scroll(root);
  act(() => {
    record.callback(
      [
        {
          isIntersecting: !above,
          boundingClientRect: { top },
          target: record.targets[0],
        } as IntersectionObserverEntry,
      ],
      record.instance,
    );
  });
}

function firePassedTop(record: ObserverRecord, _target: Element) {
  firePosition(record, true, 200);
}

function activeAutoMarkObserver() {
  const record = observerRecords.findLast(
    (record) => record.active && record.options?.threshold === 0,
  );
  expect(record).toBeDefined();
  return record!;
}

function renderAutoMarkList(overrides: Partial<ReviewDiffListProps> = {}) {
  const entry = file("a.ts");
  const props: ReviewDiffListProps = {
    files: [entry],
    reviewedFiles: new Set(),
    staleFiles: new Set(),
    sessionId: "session-1",
    autoMarkOnScroll: true,
    wordWrap: false,
    enableWalkthroughAnnotations: false,
    onToggleReviewed: vi.fn(),
    onDiscard: () => undefined,
    fileRefs: new Map([[entry.path, createRef<HTMLDivElement>()]]),
    ...overrides,
  };
  const content = () => (
    <TooltipProvider>
      <ReviewDiffList {...props} />
    </TooltipProvider>
  );
  const view = render(content());
  return {
    ...view,
    root: view.container.firstElementChild as HTMLElement,
    onToggleReviewed: props.onToggleReviewed,
    update: (next: Partial<ReviewDiffListProps>) => {
      Object.assign(props, next);
      view.rerender(content());
    },
  };
}

let scrollIntoViewDescriptor: PropertyDescriptor | undefined;

beforeEach(() => {
  observerRecords.length = 0;
  diffViewerProps.length = 0;
  installIntersectionObserver();
  scrollIntoViewDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "scrollIntoView",
  );
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    callback(0);
    return 1;
  });
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (scrollIntoViewDescriptor) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", scrollIntoViewDescriptor);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
});

describe("ReviewDiffList auto-mark", () => {
  it("ignores initial layout movement until the user scrolls", () => {
    const entry = file("a.ts");
    const onToggleReviewed = vi.fn();
    const view = render(
      <TooltipProvider>
        <ReviewDiffList
          files={[entry]}
          reviewedFiles={new Set()}
          staleFiles={new Set()}
          sessionId="session-1"
          autoMarkOnScroll
          wordWrap={false}
          enableWalkthroughAnnotations={false}
          onToggleReviewed={onToggleReviewed}
          onDiscard={() => undefined}
          fileRefs={new Map([[entry.path, createRef<HTMLDivElement>()]])}
        />
      </TooltipProvider>,
    );
    const scrollRoot = view.container.firstElementChild as HTMLElement;
    const autoMarkObserver = observerRecords.find((record) => record.options?.threshold === 0);
    expect(autoMarkObserver).toBeDefined();

    firePassedTop(autoMarkObserver!, autoMarkObserver!.targets[0]);
    expect(onToggleReviewed).not.toHaveBeenCalled();

    firePosition(autoMarkObserver!, false, 0);
    fireEvent.wheel(scrollRoot);
    firePassedTop(autoMarkObserver!, autoMarkObserver!.targets[0]);
    expect(onToggleReviewed).toHaveBeenCalledWith("a.ts", true);
  });

  it("ignores a programmatic file jump, then resumes on manual scroll input", () => {
    const files = [file("a.ts"), file("b.ts")];
    const onToggleReviewed = vi.fn();
    const refs = new Map(files.map((entry) => [entry.path, createRef<HTMLDivElement>()]));
    const view = render(
      <TooltipProvider>
        <ReviewDiffList
          files={files}
          reviewedFiles={new Set()}
          staleFiles={new Set()}
          sessionId="session-1"
          autoMarkOnScroll
          wordWrap={false}
          selectedFile="b.ts"
          enableWalkthroughAnnotations={false}
          onToggleReviewed={onToggleReviewed}
          onDiscard={() => undefined}
          fileRefs={refs}
        />
      </TooltipProvider>,
    );
    const scrollRoot = view.container.firstElementChild as HTMLElement;
    const firstAutoMarkObserver = observerRecords.find(
      (record) =>
        record.options?.threshold === 0 &&
        record.targets[0]?.parentElement?.textContent?.includes("a.ts"),
    );
    expect(firstAutoMarkObserver).toBeDefined();

    firePassedTop(firstAutoMarkObserver!, firstAutoMarkObserver!.targets[0]);
    expect(onToggleReviewed).not.toHaveBeenCalled();

    firePosition(firstAutoMarkObserver!, false, 0);
    fireEvent.wheel(scrollRoot);
    firePassedTop(firstAutoMarkObserver!, firstAutoMarkObserver!.targets[0]);
    expect(onToggleReviewed).toHaveBeenCalledWith("a.ts", true);
  });
});

describe("ReviewDiffList touch input", () => {
  function contact(target: Element, pointerType: string) {
    const event = new Event("pointerdown", { bubbles: true });
    Object.defineProperty(event, "pointerType", { value: pointerType });
    fireEvent(target, event);
  }

  it("marks a touch scroll from diff content without a touchmove event", () => {
    const view = renderAutoMarkList();
    firePosition(activeAutoMarkObserver(), false, 0);
    contact(view.getByText("a.ts"), "touch");
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledExactlyOnceWith("a.ts", true);
  });

  it("keeps mouse and pen clicks inside diff content suppressed", () => {
    const view = renderAutoMarkList();
    for (const pointerType of ["mouse", "pen"]) {
      firePosition(activeAutoMarkObserver(), false, 0);
      contact(view.getByText("a.ts"), pointerType);
      firePosition(activeAutoMarkObserver(), true, 200);
    }
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
  });
});

describe("ReviewDiffList manual clearing", () => {
  // @covers AC-UI-REVIEW-VIEWED-001.1, AC-UI-REVIEW-VIEWED-001.2
  it("keeps a manual uncheck until returning and passing the file again", () => {
    const view = renderAutoMarkList();
    firePosition(activeAutoMarkObserver(), false, 0);
    fireEvent.wheel(view.root);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledTimes(1);
    view.update({ reviewedFiles: new Set(["a.ts"]) });
    view.update({ reviewedFiles: new Set() });
    firePosition(activeAutoMarkObserver(), true, 200);
    fireEvent.wheel(view.root);
    firePosition(activeAutoMarkObserver(), true, 300);
    expect(view.onToggleReviewed).toHaveBeenCalledTimes(1);

    firePosition(activeAutoMarkObserver(), false, 0);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledTimes(2);
    expect(view.onToggleReviewed).toHaveBeenLastCalledWith("a.ts", true);
  });

  // @covers AC-UI-REVIEW-VIEWED-001.3
  it("does not mark an initial above-root sample or layout-only crossing", () => {
    const view = renderAutoMarkList();
    fireEvent.wheel(view.root);
    firePosition(activeAutoMarkObserver(), true, 0);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    firePosition(activeAutoMarkObserver(), false, 0);
    firePosition(activeAutoMarkObserver(), true, 0);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    firePosition(activeAutoMarkObserver(), false, 0);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledWith("a.ts", true);
  });

  it("tracks geometry while reviewed and uses the latest handler and eligibility", () => {
    const view = renderAutoMarkList();
    firePosition(activeAutoMarkObserver(), false, 0);
    view.update({ reviewedFiles: new Set(["a.ts"]) });
    fireEvent.wheel(view.root);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    view.update({ reviewedFiles: new Set() });
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    const latest = vi.fn();
    view.update({ onToggleReviewed: latest });
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(latest).not.toHaveBeenCalled();
    firePosition(activeAutoMarkObserver(), false, 0);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(latest).toHaveBeenCalledWith("a.ts", true);
  });

  // @covers AC-UI-REVIEW-VIEWED-001.4
  it("does not replay crossings consumed while stale or disabled", () => {
    const view = renderAutoMarkList();
    firePosition(activeAutoMarkObserver(), false, 0);
    fireEvent.wheel(view.root);
    view.update({ staleFiles: new Set(["a.ts"]) });
    firePosition(activeAutoMarkObserver(), true, 200);
    view.update({ staleFiles: new Set() });
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    view.update({ autoMarkOnScroll: false });
    expect(
      observerRecords.filter((record) => record.active && record.options?.threshold === 0),
    ).toHaveLength(0);
    view.update({ autoMarkOnScroll: true });
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    firePosition(activeAutoMarkObserver(), false, 0);
    firePosition(activeAutoMarkObserver(), true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledTimes(1);
  });

  it("ignores queued observations after unmount", () => {
    const view = renderAutoMarkList();
    const observer = activeAutoMarkObserver();
    firePosition(observer, false, 0);
    fireEvent.wheel(view.root);
    view.unmount();
    firePosition(observer, true, 200);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
  });
});

describe("ReviewDiffList layout movement", () => {
  // @covers AC-UI-REVIEW-VIEWED-001.3
  it("does not attribute a later layout crossing to an unobserved earlier scroll", () => {
    const view = renderAutoMarkList();
    const observer = activeAutoMarkObserver();
    firePosition(observer, false, 0);
    fireEvent.wheel(view.root);
    // The marker stays visible, so scrolling need not deliver an observer entry.
    view.root.scrollTop = 20;
    Object.defineProperty(observer.targets[0], "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 130 }),
    });
    fireEvent.scroll(view.root);
    // Collapsing preceding content now moves the marker above the root.
    firePosition(observer, true, 20);
    expect(view.onToggleReviewed).not.toHaveBeenCalled();
    firePosition(observer, false, 0);
    firePosition(observer, true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledExactlyOnceWith("a.ts", true);
  });
});

describe("ReviewDiffList review identity", () => {
  // @covers AC-UI-REVIEW-VIEWED-001.5
  it("does not share a pass between same-path files in different repositories", () => {
    const entries = ["repo-a", "repo-b"].map((repository_name) => ({
      ...file("a.ts"),
      repository_name,
    }));
    const view = renderAutoMarkList({
      files: entries,
      fileRefs: new Map(
        entries.map((entry) => [reviewFileKey(entry), createRef<HTMLDivElement>()]),
      ),
    });
    const records = observerRecords.filter(
      (record) => record.active && record.options?.threshold === 0,
    );
    expect(records).toHaveLength(2);
    for (const record of records) firePosition(record, false, 0);
    fireEvent.wheel(view.root);
    firePosition(records[0], true, 200);
    expect(view.onToggleReviewed).toHaveBeenCalledExactlyOnceWith(reviewFileKey(entries[0]), true);
  });
});

describe("ReviewDiffList walkthrough annotations", () => {
  it("forwards disabled walkthrough annotations to file diffs", () => {
    const entry = file("a.ts");
    render(
      <TooltipProvider>
        <ReviewDiffList
          files={[entry]}
          reviewedFiles={new Set()}
          staleFiles={new Set()}
          sessionId="session-1"
          autoMarkOnScroll={false}
          wordWrap={false}
          selectedFile="a.ts"
          enableWalkthroughAnnotations={false}
          onToggleReviewed={() => undefined}
          onDiscard={() => undefined}
          fileRefs={new Map([[entry.path, createRef<HTMLDivElement>()]])}
        />
      </TooltipProvider>,
    );

    expect(diffViewerProps.at(-1)?.enableWalkthroughAnnotations).toBe(false);
  });
});

describe("ReviewDiffList pending diffs", () => {
  it("keeps file membership visible without rendering or auto-marking an empty diff", () => {
    const entry = { ...file("pending.ts"), diff: "", diff_state: "pending" as const };
    const onToggleReviewed = vi.fn();
    render(
      <TooltipProvider>
        <ReviewDiffList
          files={[entry]}
          reviewedFiles={new Set()}
          staleFiles={new Set()}
          sessionId="session-1"
          autoMarkOnScroll
          wordWrap={false}
          enableWalkthroughAnnotations={false}
          onToggleReviewed={onToggleReviewed}
          onDiscard={() => undefined}
          fileRefs={new Map([[entry.path, createRef<HTMLDivElement>()]])}
        />
      </TooltipProvider>,
    );

    expect(document.querySelector('[data-testid="review-diff-pending"]')?.textContent).toBe(
      "Diff is loading",
    );
    expect(diffViewerProps.some((props) => props.filePath === entry.path)).toBe(false);
    expect(observerRecords.some((record) => record.options?.threshold === 0)).toBe(false);
    expect(onToggleReviewed).not.toHaveBeenCalled();
  });
});
