import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";
import type { PRFeedback, PRReview, TaskPR } from "@/lib/types/github";
import { PRDetailContent } from "./pr-detail-panel";
import { MobileReviewPanel } from "@/components/task/mobile/mobile-review-panel";
import type { ReviewItemSummary } from "@/lib/plugins/types";

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/github-api")>()),
  getPRFeedback: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));
vi.mock("@/lib/layout/panel-portal-manager", () => ({ setPanelTitle: vi.fn() }));

const pr: TaskPR = {
  id: "pr-1",
  workspace_id: "ws-1",
  task_id: "task-1",
  owner: "acme",
  repo: "demo",
  pr_number: 42,
  pr_url: "https://github.com/acme/demo/pull/42",
  pr_title: "Review decision",
  head_branch: "feature",
  base_branch: "main",
  author_login: "author",
  state: "open",
  review_state: "approved",
  checks_state: "success",
  mergeable_state: "clean",
  review_count: 1,
  pending_review_count: 0,
  comment_count: 0,
  unresolved_review_threads: 0,
  checks_total: 0,
  checks_passing: 0,
  additions: 0,
  deletions: 0,
  created_at: "",
  merged_at: null,
  closed_at: null,
  last_synced_at: null,
  updated_at: "",
};

function review(
  id: number,
  state: string,
  hour: string,
  body: string,
  author = "reviewer",
): PRReview {
  return { id, author, author_avatar: "", state, body, created_at: `2026-10-27T${hour}:00:00Z` };
}

function feedback(reviews: PRReview[], requested: string[] = []): PRFeedback {
  return {
    pr: {
      number: pr.pr_number,
      title: pr.pr_title,
      url: pr.pr_url,
      html_url: pr.pr_url,
      state: "open",
      head_branch: pr.head_branch,
      base_branch: pr.base_branch,
      author_login: pr.author_login,
      repo_owner: pr.owner,
      repo_name: pr.repo,
      draft: false,
      mergeable: true,
      additions: 0,
      deletions: 0,
      requested_reviewers: requested.map((login) => ({ login, type: "user" as const })),
      created_at: "",
      updated_at: "",
      merged_at: null,
      closed_at: null,
    },
    reviews,
    comments: [],
    checks: [],
    has_issues: false,
  };
}

function renderDetail(reviews: PRReview[], requested: string[] = []) {
  vi.mocked(getPRFeedback).mockResolvedValue(feedback(reviews, requested));
  const initialState = {
    workspaces: { activeId: "ws-1", items: [] },
  } as unknown as Partial<AppState>;
  render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          <PRDetailContent taskPR={pr} sessionId="session-1" />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

const INITIAL_APPROVAL = "Initial approval";
const CHANGES_REQUESTED = "changes requested";
const DISMISSED_REVIEW = "Dismissed review";

beforeEach(() => vi.mocked(getPRFeedback).mockReset());
afterEach(cleanup);

it("keeps the approved header after the same reviewer adds a comment", async () => {
  renderDetail([
    review(1, "APPROVED", "10", INITIAL_APPROVAL),
    review(2, "COMMENTED", "11", "A follow-up observation"),
  ]);
  expect(await screen.findByText("A follow-up observation")).toBeTruthy();
  expect(screen.getByText("approved", { exact: true })).toBeTruthy();
});

it("shows the approved header for the same approval without a follow-up", async () => {
  renderDetail([review(1, "APPROVED", "10", INITIAL_APPROVAL)]);
  expect(await screen.findByText(INITIAL_APPROVAL)).toBeTruthy();
  expect(screen.getByText("approved", { exact: true })).toBeTruthy();
});

it.each([
  [
    "change request followed by comment",
    [review(1, "CHANGES_REQUESTED", "10", "Changes"), review(2, "COMMENTED", "11", "Later note")],
    CHANGES_REQUESTED,
    "Later note",
  ],
  [
    "later approval replaces change request",
    [
      review(1, "CHANGES_REQUESTED", "10", "Changes"),
      review(2, "APPROVED", "11", "Approved later"),
    ],
    "approved",
    "Approved later",
  ],
  [
    "later change request replaces approval",
    [review(1, "APPROVED", "10", "Approval"), review(2, "CHANGES_REQUESTED", "11", "Needs work")],
    CHANGES_REQUESTED,
    "Needs work",
  ],
  [
    "a different reviewer's change request wins",
    [
      review(1, "APPROVED", "10", "Approval"),
      review(2, "CHANGES_REQUESTED", "11", "Other review", "other"),
    ],
    CHANGES_REQUESTED,
    "Other review",
  ],
  [
    "a different reviewer's neutral review does not erase approval",
    [review(1, "APPROVED", "10", "Approval"), review(2, "COMMENTED", "11", "Other note", "other")],
    "approved",
    "Other note",
  ],
  [
    "pending after approval preserves the decision",
    [review(1, "APPROVED", "10", "Approval"), review(2, "PENDING", "11", "Pending note")],
    "approved",
    "Pending note",
  ],
  [
    "neutral-only history remains pending",
    [review(1, "COMMENTED", "10", "Neutral only")],
    "pending",
    "Neutral only",
  ],
  [
    "dismissed-only history remains pending",
    [review(1, "DISMISSED", "10", DISMISSED_REVIEW)],
    "pending",
    DISMISSED_REVIEW,
  ],
  [
    "states are case-insensitive",
    [review(1, "approved", "10", "Lowercase decision")],
    "approved",
    "Lowercase decision",
  ],
  [
    "newer binding time wins when rows are out of order",
    [
      review(1, "CHANGES_REQUESTED", "11", "Newer request"),
      review(2, "APPROVED", "10", "Older approval"),
    ],
    CHANGES_REQUESTED,
    "Newer request",
  ],
  [
    "equal-time binding rows use the later source row",
    [review(1, "CHANGES_REQUESTED", "10", "First"), review(2, "APPROVED", "10", "Second")],
    "approved",
    "Second",
  ],
  [
    "author keys follow exact source casing",
    [
      review(1, "CHANGES_REQUESTED", "10", "Uppercase author", "Reviewer"),
      review(2, "APPROVED", "11", "Lowercase author", "reviewer"),
    ],
    CHANGES_REQUESTED,
    "Lowercase author",
  ],
  [
    "anonymous timestamps identify separate reviewers",
    [
      review(1, "CHANGES_REQUESTED", "10", "Anonymous change", ""),
      review(2, "APPROVED", "11", "Anonymous approval", ""),
    ],
    CHANGES_REQUESTED,
    "Anonymous approval",
  ],
])("%s", async (_name, reviews, decision, visibleBody) => {
  renderDetail(reviews);
  await screen.findByText(visibleBody);
  expect(screen.getByText(decision, { exact: true })).toBeTruthy();
});

it("keeps a requested reviewer pending without hiding a different binding approval", async () => {
  renderDetail([review(1, "APPROVED", "10", "Approval")], ["other"]);
  await screen.findByTestId("change-request-pending-reviewer-other");
  expect(screen.getByText("approved", { exact: true })).toBeTruthy();
});

it("keeps the dismissed review re-request action", async () => {
  renderDetail([review(1, "DISMISSED", "10", DISMISSED_REVIEW)]);
  await screen.findByText(DISMISSED_REVIEW);
  expect(screen.getByTestId("change-request-review-action-rerequest-review-reviewer")).toBeTruthy();
});

it("uses the linked PR fallback when there is no review or request", async () => {
  renderDetail([]);
  await screen.findByText("No reviews yet");
  expect(screen.getByText("approved", { exact: true })).toBeTruthy();
});

it("shows pending for a request without review history", async () => {
  renderDetail([], ["other"]);
  await screen.findByTestId("change-request-pending-reviewer-other");
  expect(screen.getByText("pending", { exact: true })).toBeTruthy();
});

it("renders the same decision and follow-up in the selected phone Review panel", async () => {
  const reviews = [
    review(1, "APPROVED", "10", INITIAL_APPROVAL),
    review(2, "COMMENTED", "11", "Phone follow-up"),
  ];
  vi.mocked(getPRFeedback).mockResolvedValue(feedback(reviews));
  const selected: ReviewItemSummary = {
    providerId: "github",
    reviewKey: "acme/demo/42",
    title: pr.pr_title,
    url: pr.pr_url,
    connectionScope: "https://github.com",
    repositoryId: "acme/demo",
    changeRequestNumber: 42,
    state: "OPEN",
  };
  const initialState = {
    workspaces: { activeId: "ws-1", items: [] },
    taskPRs: { byTaskId: { "task-1": [pr] } },
    tasks: {
      activeTaskId: "task-1",
      activeSessionId: "session-1",
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
  } as unknown as Partial<AppState>;
  render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>
          <MobileReviewPanel
            currentMobilePanel="review"
            reviews={[selected]}
            selectedReview={selected}
            onSelectReview={() => {}}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  expect(await screen.findByText("Phone follow-up")).toBeTruthy();
  expect(screen.getByTestId("mobile-review-panel")).toBeTruthy();
  expect(screen.getByText("approved", { exact: true })).toBeTruthy();
});
