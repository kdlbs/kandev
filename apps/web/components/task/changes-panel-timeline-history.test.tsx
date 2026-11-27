import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ChangesPanelTimelineContentProps } from "./changes-panel-timeline-types";
import { ChangesInlineCommitState } from "./changes-inline-commit-state";
import {
  useCommitSections,
  useHistoryRowRenderer,
  type CommitSection,
  type HistoryExpansionState,
} from "./changes-panel-timeline-history";

const mocks = vi.hoisted(() => ({ t: vi.fn((key: string) => key) }));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: mocks.t }) }));

const noop = () => {};
const commits = [
  {
    commit_sha: "commit-1",
    commit_message: "First commit",
    insertions: 1,
    deletions: 0,
  },
];
const prCommits: ChangesPanelTimelineContentProps["prCommits"] = [];
const emptyCommitSections: CommitSection[] = [];
const dialogs = {
  handleOpenAmendDialog: noop,
  handleOpenResetDialog: noop,
};

function timelineProps(): ChangesPanelTimelineContentProps {
  return {
    relation: { presentation: "combined" },
    commits,
    prCommits,
    hasStaged: false,
    hasUnstaged: false,
    hasPRFiles: false,
    prFiles: [],
    providerPRNumber: undefined,
    dialogs,
    onOpenDiffFile: noop,
    onOpenCommitDetail: noop,
    onRevertCommit: noop,
    onRepoPush: noop,
    onRepoCreatePR: noop,
    repoDisplayName: (name: string) => name,
    perRepoStatus: [],
    prByRepo: {},
    pushDisabled: false,
  } as unknown as ChangesPanelTimelineContentProps;
}

function historyExpansion(): HistoryExpansionState {
  return {
    defaultCollapsedByKey: new Map(),
    sectionOverrides: new Map(),
    setSectionOverrides: vi.fn(),
    collapsedRepositories: new Set(),
    setCollapsedRepositories: vi.fn(),
    collapsedInlineDirectories: new Set(),
    setCollapsedInlineDirectories: vi.fn(),
  };
}

describe("Changes panel history model memoization", () => {
  it("reuses commit sections when a parent rerenders with unchanged history inputs", () => {
    const props = timelineProps();
    const hook = renderHook(({ input }) => useCommitSections(input), {
      initialProps: { input: props },
    });
    const initialSections = hook.result.current;

    hook.rerender({ input: { ...props } });

    expect(initialSections).toHaveLength(1);
    expect(hook.result.current).toBe(initialSections);
  });

  it("keeps the row renderer stable when props identity changes but actions do not", () => {
    const props = timelineProps();
    const detailState = new ChangesInlineCommitState({
      contextKey: "task/session/environment",
      request: async () => ({}),
    });
    const expansion = historyExpansion();
    const hook = renderHook(
      ({ input }) => useHistoryRowRenderer(input, emptyCommitSections, detailState, expansion),
      { initialProps: { input: props } },
    );
    const initialRenderer = hook.result.current;

    hook.rerender({ input: { ...props } });

    expect(hook.result.current).toBe(initialRenderer);
  });
});
