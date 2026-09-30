import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import type { CommitItem } from "./commit-row";
import { ChangesTimelineHistoryRow } from "./changes-timeline-history-row";
import { buildChangesHistoryTimelineRows } from "./changes-timeline-model";
import { ChangesTimelineViewport } from "./changes-timeline-viewport";

vi.mock("@/hooks/use-copy-repository-path", () => ({
  useCopyRepositoryPath: () => vi.fn(),
}));

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

beforeEach(() => {
  resizeObservers.length = 0;
  vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
});

afterEach(() => {
  cleanup();
  resizeObservers.length = 0;
  vi.unstubAllGlobals();
});

function inlineCommitRows(target: CommitDetailTarget) {
  const commit: CommitItem = {
    commit_sha: target.sha,
    commit_message: "Keyboard navigation commit",
    insertions: 1,
    deletions: 0,
    statsAvailable: true,
    detailTarget: target,
    repository_name: target.source === "local" ? target.repo : target.repositoryName,
  };
  return {
    commit,
    rows: buildChangesHistoryTimelineRows(
      [
        {
          kind: "commits",
          sectionKey: "history",
          label: "Commits",
          testId: "commits-section",
          collapsed: false,
          commits: [
            {
              commit,
              detail: {
                expanded: true,
                status: "loaded",
                files: [
                  {
                    path: "src/file.ts",
                    status: "modified",
                    repositoryName: commit.repository_name,
                  },
                ],
              },
            },
          ],
          collapsedRepositories: new Set(),
          collapsedDirectories: new Set(),
          layout: "flat",
        },
      ],
      new Set(),
    ),
  };
}

function ExpandedCommitTimeline({
  target,
  onOpenCommitDetail,
}: {
  target: CommitDetailTarget;
  onOpenCommitDetail: (target: CommitDetailTarget, file?: { path: string; token: number }) => void;
}) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  const { commit, rows } = inlineCommitRows(target);
  return (
    <div
      ref={setScrollElement}
      data-testid="history-scroll-owner"
      style={{ height: 600, overflow: "auto" }}
    >
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={scrollElement}
        estimateSize={() => 36}
        renderRow={(row) => (
          <ChangesTimelineHistoryRow
            row={row}
            sectionCommits={[commit]}
            actions={{ onOpenDiff: vi.fn(), onOpenCommitDetail, pushDisabled: true }}
            onToggleSection={vi.fn()}
            onToggleRepository={vi.fn()}
            onToggleCommit={vi.fn()}
            onRetryCommit={vi.fn()}
            onToggleInlineDirectory={vi.fn()}
          />
        )}
      />
    </div>
  );
}

describe("ChangesTimelineHistoryRow keyboard activation", () => {
  it("opens the complete commit target once for Enter and Space after ArrowDown reaches its file", async () => {
    const target: CommitDetailTarget = {
      source: "local",
      sha: "abc123456789",
      repo: "backend",
    };
    const onOpenCommitDetail = vi.fn();
    render(<ExpandedCommitTimeline target={target} onOpenCommitDetail={onOpenCommitDetail} />);

    act(() => resizeObservers.forEach((observer) => observer.emit()));
    await waitFor(() => expect(screen.getByTestId("commit-toggle")).toBeTruthy());
    const commitToggle = screen.getByTestId("commit-toggle");
    commitToggle.focus();
    fireEvent.keyDown(commitToggle, { key: "ArrowDown" });

    const fileRow = await screen.findByTestId("commit-file-src-file.ts");
    await waitFor(() => expect(document.activeElement).toBe(fileRow));

    const expectedTokens: number[] = [];
    for (const key of ["Enter", " "]) {
      fileRow.focus();
      fireEvent.keyDown(fileRow, { key });
      expect(onOpenCommitDetail).toHaveBeenCalledTimes(expectedTokens.length + 1);
      const [actualTarget, navigation] = onOpenCommitDetail.mock.calls.at(-1) ?? [];
      expect(actualTarget).toEqual(target);
      expect(navigation?.path).toBe("src/file.ts");
      expect(navigation?.token).toEqual(expect.any(Number));
      expectedTokens.push(navigation?.token ?? 0);
    }
    expect(expectedTokens[0]).toBeGreaterThan(0);
    expect(expectedTokens[1]).toBeGreaterThan(expectedTokens[0]);
  });
});
