import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { createRef, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ReviewFile } from "./types";

type ObserverRecord = {
  callback: IntersectionObserverCallback;
  instance: IntersectionObserver;
  options?: IntersectionObserverInit;
  targets: Element[];
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
      this.record = { callback, instance: this, options, targets: [] };
      observerRecords.push(this.record);
    }

    disconnect() {}
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

function firePassedTop(record: ObserverRecord, target: Element) {
  const root = record.options?.root as HTMLElement;
  Object.defineProperty(root, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ top: 100 }),
  });
  act(() => {
    record.callback(
      [
        {
          isIntersecting: false,
          boundingClientRect: { top: 50 },
          target,
        } as IntersectionObserverEntry,
      ],
      record.instance,
    );
  });
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

    fireEvent.wheel(scrollRoot);
    firePassedTop(firstAutoMarkObserver!, firstAutoMarkObserver!.targets[0]);
    expect(onToggleReviewed).toHaveBeenCalledWith("a.ts", true);
  });

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
