import { useCallback, useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import type { FileInfo } from "@/lib/state/store";
import { ChangesInlineCommitState } from "./changes-inline-commit-state";
import type { CommitItem } from "./commit-row";
import { ChangesTimelineHistoryRow } from "./changes-timeline-history-row";
import {
  buildChangesHistoryTimelineRows,
  changesCommitExpansionKey,
} from "./changes-timeline-model";
import { ChangesTimelineViewport } from "./changes-timeline-viewport";

vi.mock("@/hooks/use-copy-repository-path", () => ({
  useCopyRepositoryPath: () => vi.fn(),
}));

const resizeObservers: ControlledResizeObserver[] = [];
const INLINE_FILE_PATH = "src/file.ts";

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
                    path: INLINE_FILE_PATH,
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

function MountedCommitHeaders({
  targets,
  state,
}: {
  targets: CommitDetailTarget[];
  state: ChangesInlineCommitState;
}) {
  const commits: CommitItem[] = targets.map((target) => ({
    commit_sha: target.sha,
    commit_message: `Commit ${target.sha}`,
    insertions: 1,
    deletions: 0,
    statsAvailable: true,
    detailTarget: target,
    repository_name: target.source === "local" ? target.repo : target.repositoryName,
  }));
  const rows = buildChangesHistoryTimelineRows(
    [
      {
        kind: "commits",
        sectionKey: "history",
        label: "Commits",
        testId: "commits-section",
        collapsed: false,
        commits: commits.map((commit) => ({
          commit,
          detail: {
            expanded: false,
            status: "loaded" as const,
            files: [
              {
                path: INLINE_FILE_PATH,
                status: "modified" as const,
                repositoryName: commit.repository_name,
              },
            ],
          },
        })),
        collapsedRepositories: new Set(),
        collapsedDirectories: new Set(),
        layout: "flat",
      },
    ],
    new Set(targets.map((target) => changesCommitExpansionKey("history", target))),
  );
  const onCommitTargetMounted = useCallback(
    (targetKey: string) => state.registerMountedTargetKey(targetKey),
    [state],
  );
  return (
    <div>
      {rows.map((row) => (
        <ChangesTimelineHistoryRow
          key={row.key}
          row={row}
          actions={{ onOpenDiff: vi.fn(), pushDisabled: true }}
          onToggleSection={vi.fn()}
          onToggleRepository={vi.fn()}
          onToggleCommit={vi.fn()}
          onRetryCommit={vi.fn()}
          onToggleInlineDirectory={vi.fn()}
          onCommitTargetMounted={onCommitTargetMounted}
        />
      ))}
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
      expect(navigation?.path).toBe(INLINE_FILE_PATH);
      expect(navigation?.token).toEqual(expect.any(Number));
      expectedTokens.push(navigation?.token ?? 0);
    }
    expect(expectedTokens[0]).toBeGreaterThan(0);
    expect(expectedTokens[1]).toBeGreaterThan(expectedTokens[0]);
  });
});

describe("mounted collapsed commit details", () => {
  it("keeps mounted headers cached and makes snapshots eviction candidates after unmount", async () => {
    const targets: CommitDetailTarget[] = ["one", "two", "three"].map((sha) => ({
      source: "local",
      sha,
      repo: "backend",
    }));
    const request = vi.fn(async () => ({
      [INLINE_FILE_PATH]: {
        path: INLINE_FILE_PATH,
        status: "modified",
        staged: false,
        additions: 1,
        deletions: 0,
      } as FileInfo,
    }));
    const state = new ChangesInlineCommitState({
      contextKey: "task/session/environment",
      request,
      maxCollapsedTargets: 1,
    });
    const view = render(<MountedCommitHeaders targets={targets.slice(0, 2)} state={state} />);

    await waitFor(() => expect(screen.getAllByTestId("commit-toggle")).toHaveLength(2));
    for (const target of targets.slice(0, 2)) {
      await act(async () => {
        await state.expand(target);
        state.collapse(target);
      });
    }
    await act(async () => state.expand(targets[0]!));
    expect(request).toHaveBeenCalledTimes(2);
    expect(state.get(targets[1]!).status).toBe("loaded");

    view.rerender(<MountedCommitHeaders targets={[targets[1]!]} state={state} />);
    await act(async () => state.collapse(targets[0]!));
    await act(async () => {
      await state.expand(targets[2]!);
      state.collapse(targets[2]!);
    });
    expect(state.get(targets[0]!).status).toBe("idle");
    await act(async () => state.expand(targets[0]!));
    expect(request).toHaveBeenCalledTimes(4);
  });
});
