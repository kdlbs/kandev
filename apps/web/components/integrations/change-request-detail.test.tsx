import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import {
  ChangeRequestDetail,
  type ChangeRequestDetailModel,
  type ChangeRequestDetailProps,
} from "./change-request-detail";

const clipboardMocks = vi.hoisted(() => ({ copyToClipboard: vi.fn() }));

vi.mock("@/lib/utils/copy-to-clipboard", () => clipboardMocks);

const REQUEST_COPY_TEST_ID = "change-request-copy-url";
const ARIA_LABEL_ATTRIBUTE = "aria-label";
const REQUEST_COPY_LABEL = "Copy pull request URL";
const REQUEST_COPIED_LABEL = "Pull request URL copied";
const COMMENT_COPIED_LABEL = "Comment URL copied";

const detail: ChangeRequestDetailModel = {
  providerId: "bitbucket",
  reviewKey: "workspace/repo#42",
  number: 42,
  title: "Fix authentication",
  url: "https://bitbucket.org/workspace/repo/pull-requests/42",
  state: "open",
  author: {
    name: "Ada Lovelace",
    url: "https://bitbucket.org/ada",
  },
  createdAt: "2026-08-06T12:00:00Z",
  sourceBranch: "feature/auth",
  targetBranch: "main",
  additions: 12,
  deletions: 3,
  description: "Use the new token exchange.",
  reviewState: "approved",
  pendingReviewCount: 1,
  reviews: [
    {
      id: "review-1",
      author: { name: "Grace", url: "https://bitbucket.org/grace" },
      state: "APPROVED",
      body: "Looks good.",
      createdAt: "2026-08-06T12:30:00Z",
    },
  ],
  requestedReviewers: [{ name: "Linus" }],
  checks: [
    {
      id: "check-1",
      name: "CI",
      state: "completed",
      conclusion: "success",
      url: "https://ci.example.test/1",
    },
  ],
  comments: [
    {
      id: "comment-1",
      author: { name: "Grace", url: "https://bitbucket.org/grace" },
      body: "Please keep this branch.",
      createdAt: "2026-08-06T12:45:00Z",
      path: "auth.go",
      line: 42,
    },
    {
      id: "comment-2",
      parentId: "comment-1",
      author: { name: "Ada" },
      body: "Done.",
      createdAt: "2026-08-06T13:00:00Z",
    },
  ],
  lastSyncedAt: "2026-08-06T13:01:00Z",
};

function renderDetail(props: Partial<ChangeRequestDetailProps> = {}) {
  return render(
    <TooltipProvider>
      <ChangeRequestDetail detail={detail} {...props} />
    </TooltipProvider>,
  );
}

afterEach(async () => {
  cleanup();
  vi.useRealTimers();
  clipboardMocks.copyToClipboard.mockReset();
  await act(async () => {
    await i18n.changeLanguage("en");
  });
});

describe("ChangeRequestDetail", () => {
  it("renders the GitHub detail anatomy from provider-neutral data", () => {
    renderDetail();

    expect(screen.getByRole("link", { name: "Fix authentication" }).getAttribute("href")).toBe(
      detail.url,
    );
    expect(screen.getByRole("link", { name: "Ada Lovelace" }).getAttribute("href")).toBe(
      detail.author.url,
    );
    expect(screen.getByText("#42")).toBeTruthy();
    expect(screen.getByText("feature/auth")).toBeTruthy();
    expect(screen.getByText("main")).toBeTruthy();
    expect(screen.getByText(/Reviews/)).toBeTruthy();
    expect(screen.getByText(/Checks/)).toBeTruthy();
    expect(screen.getByText(/Comments/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Description/ }));
    expect(screen.getByText("Use the new token exchange.")).toBeTruthy();
    expect(screen.getByRole("link", { name: "CI" }).getAttribute("href")).toBe(
      detail.checks[0].url,
    );
    expect(screen.getByText("auth.go:42")).toBeTruthy();
    expect(screen.getByText("Done.")).toBeTruthy();
    expect(screen.queryByRole("tab")).toBeNull();
  });
});

describe("ChangeRequestDetail interactions", () => {
  it("preserves GitHub section summaries and bulk/thread context actions", () => {
    const onAddContext = vi.fn();
    renderDetail({
      detail: {
        ...detail,
        requestedReviewers: [{ name: "platform", kind: "team" }],
        checks: [
          {
            ...detail.checks[0],
            conclusion: "failure",
            output: "tests failed",
          },
        ],
      },
      onAddContext,
    });

    expect(screen.getByText("Reviews: 1 approved, 1 pending")).toBeTruthy();
    expect(screen.getByText("CI Checks: 1 failed")).toBeTruthy();
    expect(screen.getByText("platform (team)")).toBeTruthy();

    const addAll = screen.getAllByRole("button", { name: "Add all" });
    expect(addAll).toHaveLength(3);
    addAll.forEach((button) => fireEvent.click(button));
    expect(onAddContext).toHaveBeenCalledWith("review", expect.stringContaining("All PR Reviews"));
    expect(onAddContext).toHaveBeenCalledWith(
      "review",
      expect.stringContaining("**Grace**: Approved"),
    );
    expect(onAddContext).toHaveBeenCalledWith(
      "check",
      expect.stringContaining("1 CI Check Failed"),
    );
    expect(onAddContext).toHaveBeenCalledWith("check", expect.stringContaining("**CI**: failure"));
    expect(onAddContext).toHaveBeenCalledWith(
      "comment",
      expect.stringContaining("All PR Comments"),
    );

    const threadContext = screen.getByTestId("change-request-thread-context-comment-1");
    fireEvent.click(threadContext.querySelector("button")!);
    expect(onAddContext).toHaveBeenCalledWith("comment", expect.stringContaining("Comment Thread"));
  });

  it("shows provider aggregate pending review count when identities are unavailable", () => {
    renderDetail({
      detail: {
        ...detail,
        reviews: [],
        requestedReviewers: [],
        pendingReviewCount: 2,
      },
    });

    expect(screen.getByText("Reviews: 2 pending")).toBeTruthy();
  });
});

describe("ChangeRequestDetail copy actions", () => {
  it("copies the request and comment permalinks with transient confirmation", async () => {
    clipboardMocks.copyToClipboard.mockResolvedValue(true);
    const detailWithUrls = {
      ...detail,
      state: "merged",
      comments: detail.comments.map((comment) => ({
        ...comment,
        url: `https://bitbucket.org/workspace/repo/pull-requests/42#${comment.id}`,
      })),
    } as unknown as ChangeRequestDetailModel;

    renderDetail({ detail: detailWithUrls, presentation: "mobile" });

    const requestCopy = screen.getByTestId(REQUEST_COPY_TEST_ID);
    const rootCopy = screen.getByTestId("change-request-comment-copy-comment-1");
    const replyCopy = screen.getByTestId("change-request-comment-copy-comment-2");

    fireEvent.click(requestCopy);
    fireEvent.click(rootCopy);
    fireEvent.click(replyCopy);

    await waitFor(() => {
      expect(clipboardMocks.copyToClipboard).toHaveBeenNthCalledWith(1, detailWithUrls.url);
      expect(clipboardMocks.copyToClipboard).toHaveBeenNthCalledWith(
        2,
        "https://bitbucket.org/workspace/repo/pull-requests/42#comment-1",
      );
      expect(clipboardMocks.copyToClipboard).toHaveBeenNthCalledWith(
        3,
        "https://bitbucket.org/workspace/repo/pull-requests/42#comment-2",
      );
      expect(requestCopy.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(REQUEST_COPIED_LABEL);
      expect(rootCopy.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(COMMENT_COPIED_LABEL);
    });
  });

  it("omits copy actions when a request or comment URL is unavailable", () => {
    const detailWithoutUrls = {
      ...detail,
      url: "",
      comments: detail.comments.map(({ ...comment }) => {
        delete (comment as { url?: string }).url;
        return comment;
      }),
    } as unknown as ChangeRequestDetailModel;

    renderDetail({ detail: detailWithoutUrls });

    expect(screen.queryByTestId(REQUEST_COPY_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("change-request-comment-copy-comment-1")).toBeNull();
    expect(screen.queryByTestId("change-request-comment-copy-comment-2")).toBeNull();
  });

  it("does not show copied confirmation when the clipboard write fails", async () => {
    clipboardMocks.copyToClipboard.mockResolvedValue(false);
    renderDetail();

    const requestCopy = screen.getByTestId(REQUEST_COPY_TEST_ID);
    fireEvent.click(requestCopy);

    await waitFor(() => expect(clipboardMocks.copyToClipboard).toHaveBeenCalledWith(detail.url));
    expect(requestCopy.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(REQUEST_COPY_LABEL);
  });

  it("updates copy labels when the locale changes", async () => {
    renderDetail();
    const requestCopy = screen.getByTestId(REQUEST_COPY_TEST_ID);
    expect(requestCopy.getAttribute(ARIA_LABEL_ATTRIBUTE)).toBe(REQUEST_COPY_LABEL);

    await act(async () => {
      await i18n.changeLanguage("pseudo");
    });

    expect(requestCopy.getAttribute(ARIA_LABEL_ATTRIBUTE)).toContain("ƥũĺĺ");
  });
});

describe("ChangeRequestDetail actions and states", () => {
  it("dispatches only advertised provider actions with mobile-safe targets", () => {
    const onAction = vi.fn();
    renderDetail({
      presentation: "mobile",
      actions: [
        { id: "approve", label: "Approve", placement: "header", tone: "success" },
        { id: "decline", label: "Decline", placement: "header", tone: "danger" },
        { id: "comment", label: "Comment", placement: "comment", input: "text" },
        { id: "reply", label: "Reply", placement: "thread", input: "text" },
      ],
      onAction,
    });

    const approve = screen.getByRole("button", { name: "Approve" });
    expect(approve.classList.contains("min-h-11")).toBe(true);
    fireEvent.click(approve);
    expect(onAction).toHaveBeenCalledWith({ actionId: "approve" });

    fireEvent.change(screen.getByLabelText("Add review comment"), {
      target: { value: "Ship it." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Comment" }));
    expect(onAction).toHaveBeenCalledWith({ actionId: "comment", body: "Ship it." });

    fireEvent.change(screen.getByRole("textbox", { name: "Reply to thread comment-1" }), {
      target: { value: "Resolved." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Reply to thread comment-1" }));
    expect(onAction).toHaveBeenCalledWith({
      actionId: "reply",
      body: "Resolved.",
      threadId: "comment-1",
    });
  });

  it("renders one context action for a failed check", () => {
    renderDetail({
      detail: {
        ...detail,
        reviews: [],
        comments: [],
        checks: [{ ...detail.checks[0], conclusion: "failure" }],
      },
      onAddContext: vi.fn(),
    });

    const checkRow = screen.getByRole("link", { name: "CI" }).parentElement;
    expect(checkRow?.querySelectorAll("button")).toHaveLength(1);
  });

  it("updates a running check duration", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-06T14:00:00Z"));
    renderDetail({
      detail: {
        ...detail,
        checks: [
          {
            ...detail.checks[0],
            state: "in_progress",
            conclusion: "",
            startedAt: "2026-08-06T13:58:00Z",
          },
        ],
      },
    });
    const duration = screen.getByTestId("check-duration-CI");
    const initial = duration.textContent;

    act(() => vi.advanceTimersByTime(1_000));

    expect(duration.textContent).not.toBe(initial);
    expect(duration.textContent).toContain("running");
  });

  it("owns one vertical scroller and keeps loading and error states inside it", () => {
    const { rerender } = renderDetail({ detail: null, loading: true });

    expect(screen.getByTestId("change-request-detail").classList.contains("h-full")).toBe(true);
    expect(screen.getByTestId("change-request-detail").classList.contains("min-h-0")).toBe(true);
    expect(screen.getAllByTestId("change-request-detail-scroll")).toHaveLength(1);
    expect(
      screen.getByTestId("change-request-detail-content").classList.contains("overflow-x-hidden"),
    ).toBe(true);
    expect(screen.getByLabelText("Loading change request")).toBeTruthy();

    rerender(
      <TooltipProvider>
        <ChangeRequestDetail detail={null} error="Provider unavailable" onRetry={vi.fn()} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("alert").textContent).toContain("Provider unavailable");
    expect(screen.getByRole("button", { name: "Retry" }).classList.contains("min-h-11")).toBe(true);
  });
});
