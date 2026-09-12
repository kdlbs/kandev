import type { ReactNode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  contributionHistoryExplanationKey,
  resetContributionHistoryExplanationForTests,
} from "@/hooks/domains/session/use-contribution-history-explanation";
import { requestContributionComparison } from "../remote-contribution-comparison";
import { MobileChangesPanel } from "./mobile-changes-panel";

const mocks = vi.hoisted(() => ({
  data: null as Record<string, unknown> | null,
  headerProps: null as Record<string, unknown> | null,
  comparisonTokens: [] as Array<number | undefined>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ tasks: { activeSessionId: "session-1" } }),
}));

vi.mock("@/hooks/domains/session/use-review-sources", () => ({
  useReviewSources: () => ({ sourceCounts: {} }),
}));

vi.mock("@/hooks/domains/session/use-request-changes-walkthrough", () => ({
  useRequestChangesWalkthrough: () => vi.fn(),
}));

vi.mock("../changes-panel", () => ({
  ChangesPanelBody: (props: { comparisonRequestToken?: number }) => {
    mocks.comparisonTokens.push(props.comparisonRequestToken);
    return <div data-testid="mobile-changes-body" />;
  },
  ChangesPanelHeader: (props: Record<string, unknown>) => {
    mocks.headerProps = props;
    return <div data-testid="mobile-changes-header" />;
  },
  useChangesPanelData: () => mocks.data,
  buildChangesPanelBodyProps: () => ({}),
}));

vi.mock("../changes-panel-header", () => ({
  ChangesPanelHeader: (props: Record<string, unknown>) => {
    mocks.headerProps = props;
    return <div data-testid="mobile-changes-header" />;
  },
}));

vi.mock("../panel-primitives", () => ({
  PanelRoot: ({ children, ...props }: { children: ReactNode }) => (
    <section {...props}>{children}</section>
  ),
}));

vi.mock("./mobile-diff-sheet", () => ({
  MobileDiffSheet: () => null,
}));

const target = {
  sessionId: "session-1",
  workspaceId: "workspace-1",
  repositoryScope: "frontend",
  branch: "feature/one",
  selectedPRKey: "acme/frontend/42",
  expectedLocalHead: "a".repeat(40),
  expectedRemoteHead: "b".repeat(40),
};

function makeData() {
  return {
    activeTaskId: "task-1",
    activeSessionId: "session-1",
    relation: { kind: "diverged", action: "diverged_replace" },
    contributionHistoryTarget: target,
    resolution: {},
    resolutionTarget: {},
    selectedPR: { pr_url: "https://github.com/acme/frontend/pull/42", pr_number: 42 },
    existingPrUrl: undefined,
    git: {
      hasChanges: false,
      hasCommits: true,
      hasAnything: true,
      hasUnstaged: false,
      hasStaged: false,
      pullBehind: 0,
      isLoading: false,
      loadingOperation: null,
      repoNames: ["frontend"],
      perRepoStatus: [],
      branch: "feature/one",
      comparisonTargets: [],
      cumulativeDiff: null,
      statusLoaded: true,
      unstagedFiles: [],
      stagedFiles: [],
      allFiles: [],
    },
    baseBranchDisplay: "main",
    baseBranchByRepo: {},
    pullDisabled: false,
    pullDisabledReason: undefined,
    gitCredentialDisplay: undefined,
    repoCallbacks: {
      onRepoPull: vi.fn(),
      onRepoRebase: vi.fn(),
      onRepoMerge: vi.fn(),
    },
    repoDisplayName: vi.fn(),
    walkthroughRequestReady: false,
  };
}

afterEach(() => {
  cleanup();
  resetContributionHistoryExplanationForTests();
});

beforeEach(() => {
  mocks.data = makeData();
  mocks.headerProps = null;
  mocks.comparisonTokens = [];
});

describe("MobileChangesPanel contribution comparison", () => {
  it("passes the selected contribution identity to the header and opens both histories", () => {
    render(
      <MobileChangesPanel selectedDiff={null} onClearSelected={vi.fn()} onOpenFile={vi.fn()} />,
    );

    expect(mocks.headerProps?.contributionHistoryTarget).toEqual(target);

    const key = contributionHistoryExplanationKey(target);
    expect(key).not.toBeNull();
    act(() => {
      requestContributionComparison(key!);
    });

    expect(screen.getByTestId("mobile-changes-panel")).toBeTruthy();
    expect(mocks.comparisonTokens).toContainEqual(expect.any(Number));
  });
});
