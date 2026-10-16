import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createElement, useEffect } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { afterEach, describe, it, expect, vi } from "vitest";
import { PRDetailContent, shouldShowReRequestReviewAction } from "./pr-detail-panel";
import {
  clearPRReviewRequestRegistryForTests,
  usePRScopedReviewRequest,
} from "./use-pr-scoped-review-request";
import type { TaskPR, PRFeedback, GitHubPR, PRReview } from "@/lib/types/github";
import { activateLocale, i18n } from "@/lib/i18n";

const feedbackMocks = vi.hoisted(() => ({
  refresh: vi.fn(),
  value: null as PRFeedback | null,
}));
const reviewMocks = vi.hoisted(() => ({ requestReviewers: vi.fn(), submitReview: vi.fn() }));
const RE_REQUEST_BUTTON = "change-request-review-action-rerequest-review-octocat";
const PENDING_REVIEWER = "change-request-pending-reviewer-octocat";
const DISMISSED_REVIEWED_AT = "2026-01-01T00:00:00Z";
const WORKSPACE_ID = "workspace-1";

vi.mock("@/hooks/domains/github/use-pr-feedback", () => ({
  usePRFeedback: () => ({
    feedback: feedbackMocks.value,
    loading: false,
    refresh: feedbackMocks.refresh,
  }),
}));
vi.mock("@/lib/api/domains/github-review-api", () => ({
  requestPRReviewers: reviewMocks.requestReviewers,
  submitPRReview: reviewMocks.submitReview,
}));
vi.mock("@/lib/state/slices/comments", () => ({
  isPRFeedbackComment: () => false,
  useCommentsStore: (
    selector: (state: {
      addComment: () => void;
      pendingForChat: string[];
      byId: object;
    }) => unknown,
  ) => selector({ addComment: vi.fn(), pendingForChat: [], byId: {} }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));
vi.mock("./pr-merge-button", () => ({ PRMergeButton: () => null }));
vi.mock("./pr-mergeability-notice", () => ({
  PRMergeabilityNotice: () => null,
  buildConflictResolutionMessage: () => "",
}));
vi.mock("./pr-checks-section", () => ({ ChecksSection: () => null }));
vi.mock("./pr-comments-section", () => ({ CommentsSection: () => null }));

afterEach(async () => {
  vi.useRealTimers();
  cleanup();
  feedbackMocks.refresh.mockReset();
  feedbackMocks.value = null;
  reviewMocks.requestReviewers.mockReset();
  reviewMocks.submitReview.mockReset();
  clearPRReviewRequestRegistryForTests();
  await activateLocale("en");
});

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    workspace_id: WORKSPACE_ID,
    task_id: "task",
    owner: "o",
    repo: "r",
    pr_number: 1,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
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
    ...overrides,
  };
}

function makeGitHubPR(overrides: Partial<GitHubPR> = {}): GitHubPR {
  return {
    number: 1,
    title: "Test PR",
    url: "",
    html_url: "",
    state: "open",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    repo_owner: "o",
    repo_name: "r",
    draft: false,
    mergeable: true,
    additions: 0,
    deletions: 0,
    ...overrides,
  } as GitHubPR;
}

function makeFeedback(
  overrides: {
    pr?: Partial<GitHubPR>;
    reviews?: PRReview[];
  } = {},
): PRFeedback {
  return {
    pr: makeGitHubPR(overrides.pr),
    reviews: overrides.reviews ?? [],
    comments: [],
    checks: [],
    has_issues: false,
  };
}

function ReviewRequestHarness({
  onReady,
  requestedReviewers = [],
  reviews = [],
  workspaceId = WORKSPACE_ID,
  githubHost,
}: {
  githubHost?: string;
  onReady: (reviewRequest: ReturnType<typeof usePRScopedReviewRequest>) => void;
  requestedReviewers?: { login: string; type: "user" }[];
  reviews?: PRReview[];
  workspaceId?: string | null;
}) {
  const reviewRequest = usePRScopedReviewRequest(makeTaskPR({ github_host: githubHost }), {
    workspaceId,
    requestedReviewers,
    reviews,
    refresh: vi.fn(),
    toast: vi.fn(),
  });
  useEffect(() => onReady(reviewRequest), [onReady, reviewRequest]);
  return null;
}

function dismissedReview(id = 1): PRReview {
  return {
    id,
    author: "octocat",
    author_avatar: "",
    state: "DISMISSED",
    body: "",
    created_at: DISMISSED_REVIEWED_AT,
  };
}

const TEST_INITIAL_STATE = {
  workspaces: { activeId: WORKSPACE_ID },
} as unknown as Partial<AppState>;

function prDetailTree(taskPR = makeTaskPR()) {
  return createElement(StateProvider, {
    initialState: TEST_INITIAL_STATE,
    children: createElement(
      TooltipProvider,
      undefined,
      createElement(PRDetailContent, { taskPR, sessionId: "session-1" }),
    ),
  });
}

function renderPRDetail(taskPR = makeTaskPR()) {
  return render(prDetailTree(taskPR));
}

describe("PRDetailContent shared presentation", () => {
  it("renders GitHub through the provider-neutral change request detail", () => {
    feedbackMocks.value = makeFeedback({ pr: { body: "Shared review body" } });

    renderPRDetail();

    expect(screen.getByTestId("change-request-detail")).not.toBeNull();
    expect(screen.getByText("Test PR")).not.toBeNull();
  });

  it("localizes review actions instead of embedding English in the detail model", async () => {
    await activateLocale("pseudo");
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });

    renderPRDetail();

    expect(screen.getByTestId(RE_REQUEST_BUTTON).textContent).toBe(
      i18n.t("github:reRequestReview"),
    );
    expect(screen.getByTestId(RE_REQUEST_BUTTON).textContent).not.toBe("Re-request review");
  });
});

describe("shouldShowReRequestReviewAction", () => {
  it("only permits dismissed reviews on an open PR", () => {
    expect(shouldShowReRequestReviewAction("open", "DISMISSED")).toBe(true);
    expect(shouldShowReRequestReviewAction("open", "APPROVED")).toBe(false);
    expect(shouldShowReRequestReviewAction("closed", "DISMISSED")).toBe(false);
    expect(shouldShowReRequestReviewAction("merged", "DISMISSED")).toBe(false);
  });
});

describe("PRDetailContent deferred re-request", () => {
  it("keeps a successfully re-requested reviewer pending when feedback refresh is deferred", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });

    renderPRDetail();

    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });

    await waitFor(() => {
      expect(screen.getByTestId(PENDING_REVIEWER)).not.toBeNull();
    });
    expect(reviewMocks.requestReviewers).toHaveBeenCalledWith("o", "r", 1, ["octocat"], {
      workspaceId: WORKSPACE_ID,
      expectedHost: "github.com",
    });
    expect(screen.queryByTestId(RE_REQUEST_BUTTON)).toBeNull();
  });
});

describe("PRDetailContent remount lifecycle", () => {
  it("keeps an in-flight request locked across a same-PR remount", () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockReturnValue(new Promise(() => undefined));

    const firstPanel = renderPRDetail();
    fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    firstPanel.unmount();

    renderPRDetail();
    fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));

    expect(reviewMocks.requestReviewers).toHaveBeenCalledOnce();
  });
});

describe("PRDetailContent optimistic review reconciliation", () => {
  it("reclaims optimism when a newer submitted review bypasses requested-reviewer feedback", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });

    const view = renderPRDetail();
    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });
    feedbackMocks.value = makeFeedback({
      reviews: [
        {
          id: 2,
          author: "octocat",
          author_avatar: "",
          state: "APPROVED",
          body: "Looks good",
          created_at: "2026-01-02T00:00:00Z",
        },
      ],
    });
    view.rerender(prDetailTree());

    await waitFor(() => {
      expect(screen.queryByTestId(PENDING_REVIEWER)).toBeNull();
    });
    expect(screen.getByTestId("change-request-submitted-review-octocat")).not.toBeNull();
  });
});

describe("PRDetailContent same-timestamp review reconciliation", () => {
  it("uses the review ID to resolve same-timestamp reviewer state", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    const view = renderPRDetail();
    await act(async () => fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON)));

    feedbackMocks.value = makeFeedback({
      reviews: [
        dismissedReview(),
        {
          id: 2,
          author: "octocat",
          author_avatar: "",
          state: "APPROVED",
          body: "Looks good",
          created_at: DISMISSED_REVIEWED_AT,
        },
      ],
    });
    view.rerender(prDetailTree());

    await waitFor(() => expect(screen.queryByTestId(PENDING_REVIEWER)).toBeNull());
    expect(screen.getByTestId("change-request-submitted-review-octocat")).not.toBeNull();
  });
});

describe("PRDetailContent confirmed review request", () => {
  it("reclaims confirmed pending state from the persistent registry", async () => {
    feedbackMocks.value = makeFeedback({
      reviews: [dismissedReview()],
    });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    const view = renderPRDetail();
    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });

    feedbackMocks.value = makeFeedback({
      pr: { requested_reviewers: [{ login: "octocat", type: "user" }] },
      reviews: [
        {
          id: 1,
          author: "octocat",
          author_avatar: "",
          state: "DISMISSED",
          body: "",
          created_at: DISMISSED_REVIEWED_AT,
        },
      ],
    });
    view.rerender(prDetailTree());
    await waitFor(() => {
      expect(screen.getByTestId(PENDING_REVIEWER)).not.toBeNull();
    });

    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    view.rerender(prDetailTree());

    await waitFor(() => {
      expect(screen.getByTestId(RE_REQUEST_BUTTON)).not.toBeNull();
    });
  });
});

describe("PRDetailContent same-tick re-request", () => {
  it("posts once when invoked twice before React has a chance to re-render", () => {
    reviewMocks.requestReviewers.mockReturnValue(new Promise(() => undefined));
    let reRequest: (reviewer: string) => Promise<void> = async () => undefined;

    render(
      createElement(ReviewRequestHarness, {
        onReady: (request) => {
          reRequest = request.reRequest;
        },
      }),
    );
    act(() => {
      void reRequest("octocat");
      void reRequest("octocat");
    });

    expect(reviewMocks.requestReviewers).toHaveBeenCalledOnce();
  });
});

describe("PRDetailContent workspace-scoped requests", () => {
  it("does not issue a cross-workspace request when no active workspace is available", async () => {
    let reRequest: (reviewer: string) => Promise<void> = async () => undefined;
    render(
      createElement(ReviewRequestHarness, {
        workspaceId: null,
        onReady: (request) => {
          reRequest = request.reRequest;
        },
      }),
    );

    await act(async () => reRequest("octocat"));

    expect(reviewMocks.requestReviewers).not.toHaveBeenCalled();
  });

  it("keeps same-PR requests and optimistic reconciliation isolated by workspace", async () => {
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    let reviewRequest: ReturnType<typeof usePRScopedReviewRequest> | undefined;
    const onReady = (value: ReturnType<typeof usePRScopedReviewRequest>) => {
      reviewRequest = value;
    };
    const view = render(
      createElement(ReviewRequestHarness, { onReady, workspaceId: "workspace-a" }),
    );

    await act(async () => reviewRequest?.reRequest("octocat"));
    expect(reviewRequest?.requestedReviewers).toEqual([{ login: "octocat", type: "user" }]);

    view.rerender(createElement(ReviewRequestHarness, { onReady, workspaceId: "workspace-b" }));
    expect(reviewRequest?.requestedReviewers).toEqual([]);

    await act(async () => reviewRequest?.reRequest("octocat"));
    expect(reviewMocks.requestReviewers).toHaveBeenNthCalledWith(1, "o", "r", 1, ["octocat"], {
      workspaceId: "workspace-a",
      expectedHost: "github.com",
    });
    expect(reviewMocks.requestReviewers).toHaveBeenNthCalledWith(2, "o", "r", 1, ["octocat"], {
      workspaceId: "workspace-b",
      expectedHost: "github.com",
    });

    view.rerender(
      createElement(ReviewRequestHarness, {
        onReady,
        workspaceId: "workspace-b",
        requestedReviewers: [{ login: "octocat", type: "user" }],
      }),
    );
    view.rerender(createElement(ReviewRequestHarness, { onReady, workspaceId: "workspace-a" }));

    expect(reviewRequest?.requestedReviewers).toEqual([{ login: "octocat", type: "user" }]);
  });
});

describe("PRDetailContent persistent request expiry", () => {
  it("keeps an unresolved request locked beyond the optimistic expiry window", () => {
    vi.useFakeTimers();
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    const deferred = new Promise(() => undefined);
    reviewMocks.requestReviewers.mockReturnValue(deferred);

    const view = renderPRDetail();
    fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    act(() => vi.advanceTimersByTime(5 * 60 * 1000));
    view.rerender(prDetailTree());

    expect(screen.getByTestId(RE_REQUEST_BUTTON).hasAttribute("disabled")).toBe(true);
    vi.useRealTimers();
  });

  it("expires successful optimism and updates the panel without a rerender", async () => {
    vi.useFakeTimers();
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    renderPRDetail();
    await act(async () => fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON)));

    act(() => vi.advanceTimersByTime(5 * 60 * 1000));

    expect(screen.getByTestId(RE_REQUEST_BUTTON).hasAttribute("disabled")).toBe(false);
    vi.useRealTimers();
  });

  it("clears the expiry timer when feedback confirms the request", async () => {
    vi.useFakeTimers();
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    let reviewRequest: ReturnType<typeof usePRScopedReviewRequest> | undefined;
    const onReady = (value: ReturnType<typeof usePRScopedReviewRequest>) => {
      reviewRequest = value;
    };
    const view = render(
      createElement(ReviewRequestHarness, {
        onReady,
        reviews: [dismissedReview()],
      }),
    );

    const timersBeforeRequest = vi.getTimerCount();
    await act(async () => reviewRequest?.reRequest("octocat"));
    expect(vi.getTimerCount()).toBe(timersBeforeRequest + 1);

    view.rerender(
      createElement(ReviewRequestHarness, {
        onReady,
        requestedReviewers: [{ login: "octocat", type: "user" }],
      }),
    );
    await act(async () => undefined);

    expect(vi.getTimerCount()).toBe(timersBeforeRequest);
    vi.useRealTimers();
  });

  it("reschedules to the next optimistic expiry after the first expires", async () => {
    vi.useFakeTimers();
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });
    let reviewRequest: ReturnType<typeof usePRScopedReviewRequest> | undefined;
    render(
      createElement(ReviewRequestHarness, {
        onReady: (value) => {
          reviewRequest = value;
        },
      }),
    );

    await act(async () => reviewRequest?.reRequest("octocat"));
    act(() => vi.advanceTimersByTime(60 * 1000));
    await act(async () => reviewRequest?.reRequest("hubot"));
    act(() => vi.advanceTimersByTime(4 * 60 * 1000));

    expect(reviewRequest?.requestedReviewers).toEqual([{ login: "hubot", type: "user" }]);
    vi.useRealTimers();
  });
});

describe("PRDetailContent request operation ownership", () => {
  it("does not let an old completion change a newer request", async () => {
    let resolveFirst: () => void = () => undefined;
    const firstRequest = new Promise<void>((resolve) => {
      resolveFirst = resolve;
    });
    const secondRequest = new Promise(() => undefined);
    reviewMocks.requestReviewers
      .mockReturnValueOnce(firstRequest)
      .mockReturnValueOnce(secondRequest);
    let reviewRequest: ReturnType<typeof usePRScopedReviewRequest> | undefined;
    const onReady = (value: ReturnType<typeof usePRScopedReviewRequest>) => {
      reviewRequest = value;
    };
    const view = render(createElement(ReviewRequestHarness, { onReady }));

    act(() => void reviewRequest?.reRequest("octocat"));
    view.rerender(
      createElement(ReviewRequestHarness, {
        onReady,
        requestedReviewers: [{ login: "octocat", type: "user" }],
      }),
    );
    view.rerender(createElement(ReviewRequestHarness, { onReady }));
    act(() => void reviewRequest?.reRequest("octocat"));
    await act(async () => resolveFirst());

    expect(reviewRequest?.requestingReviewers).toEqual(["octocat"]);
  });
});

describe("PRDetailContent failed feedback refresh", () => {
  it("keeps the successful re-request optimistic", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    feedbackMocks.refresh.mockImplementation(() => {
      throw new Error("Feedback refresh failed");
    });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });

    renderPRDetail();
    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });

    expect(screen.getByTestId(PENDING_REVIEWER)).not.toBeNull();
  });
});

describe("PRDetailContent failed re-request", () => {
  it("clears failed writes so the dismissed reviewer can be retried", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockRejectedValue(new Error("GitHub denied request"));

    renderPRDetail();

    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });
    await waitFor(() => {
      expect(screen.getByTestId(RE_REQUEST_BUTTON).hasAttribute("disabled")).toBe(false);
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });
    expect(reviewMocks.requestReviewers).toHaveBeenCalledTimes(2);
  });
});

describe("PRDetailContent PR identity changes", () => {
  it("does not retain optimistic reviewers after the repository PR identity changes", async () => {
    feedbackMocks.value = makeFeedback({ reviews: [dismissedReview()] });
    reviewMocks.requestReviewers.mockResolvedValue({ requested: true });

    const view = renderPRDetail();
    await act(async () => {
      fireEvent.click(screen.getByTestId(RE_REQUEST_BUTTON));
    });
    await waitFor(() => {
      expect(screen.getByTestId(PENDING_REVIEWER)).not.toBeNull();
    });

    view.rerender(prDetailTree(makeTaskPR({ pr_number: 2 })));

    expect(screen.getByTestId(RE_REQUEST_BUTTON)).not.toBeNull();
  });
});

it("isolates historical review requests when the same workspace changes GitHub hosts", async () => {
  let releaseOld: (value: { requested: boolean }) => void = () => {};
  reviewMocks.requestReviewers
    .mockReturnValueOnce(
      new Promise((resolve) => {
        releaseOld = resolve;
      }),
    )
    .mockResolvedValue({ requested: true });
  let current: ReturnType<typeof usePRScopedReviewRequest> | undefined;
  const onReady = (value: ReturnType<typeof usePRScopedReviewRequest>) => {
    current = value;
  };
  const view = render(
    createElement(ReviewRequestHarness, { onReady, githubHost: "git.company.example" }),
  );
  act(() => {
    void current?.reRequest("octocat");
  });
  expect(current?.requestingReviewers).toEqual(["octocat"]);
  view.rerender(
    createElement(ReviewRequestHarness, { onReady, githubHost: "other.company.example" }),
  );
  expect(current?.requestingReviewers).toEqual([]);
  expect(current?.requestedReviewers).toEqual([]);
  await act(async () => {
    await current?.reRequest("octocat");
  });
  expect(reviewMocks.requestReviewers).toHaveBeenNthCalledWith(2, "o", "r", 1, ["octocat"], {
    workspaceId: WORKSPACE_ID,
    expectedHost: "other.company.example",
  });
  await act(async () => releaseOld({ requested: true }));
  expect(current?.requestedReviewers).toEqual([{ login: "octocat", type: "user" }]);
});
