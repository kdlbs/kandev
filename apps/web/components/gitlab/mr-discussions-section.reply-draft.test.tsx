import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { useMRActions } from "@/hooks/domains/gitlab/use-mr-actions";
import { useMRFeedback } from "@/hooks/domains/gitlab/use-mr-feedback";
import { createMRDiscussionNote, resolveMRDiscussion } from "@/lib/api/domains/gitlab-api";
import type { GitLabMRDiscussion, GitLabMRFeedback, GitLabMRNote } from "@/lib/types/gitlab";
import { MRDiscussionsSection } from "./mr-discussions-section";

const identity = {
  workspaceId: "draft-workspace",
  host: "https://review.example.test",
  project: "team/service",
  iid: 23,
};
const mrUrl = "https://review.example.test/team/service/-/merge_requests/23";
const submitted = "First reply about error handling";
const continued = "Second unsent reply\n  with more findings  ";
const successNotice = "Reply added";
const timestamp = "2026-10-08T10:00:00Z";

function discussion(id: string): GitLabMRDiscussion {
  return {
    id,
    resolvable: true,
    resolved: false,
    path: "src/service.ts",
    line: 19,
    old_line: 0,
    created_at: timestamp,
    updated_at: timestamp,
    notes: [
      {
        id: 1,
        author: "reviewer",
        body: `Review ${id}`,
        created_at: timestamp,
        updated_at: timestamp,
      },
    ],
  };
}

type HeldResponse = {
  promise: Promise<Response>;
  resolve: (response: Response) => void;
  settled: boolean;
};
type PostedReply = { project: string; iid: number; discussion_id: string; body: string };
let held: HeldResponse[];
let posts: { body: PostedReply; response: HeldResponse }[];
let refreshes: HeldResponse[];
let discussions: GitLabMRDiscussion[];
let feedbackReads: number;
let holdRefresh: boolean;

function deferredResponse(): HeldResponse {
  let finish!: (response: Response) => void;
  const response: HeldResponse = {
    promise: new Promise<Response>((resolve) => {
      finish = resolve;
    }),
    resolve: (value) => {
      response.settled = true;
      finish(value);
    },
    settled: false,
  };
  held.push(response);
  return response;
}

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function feedback(): GitLabMRFeedback {
  return {
    mr: {
      id: 23,
      iid: 23,
      project_id: 12,
      title: "Review error handling",
      url: mrUrl,
      web_url: mrUrl,
      state: "opened",
      head_branch: "fix/errors",
      head_sha: "abc123",
      base_branch: "main",
      author_username: "author",
      project_namespace: "team",
      project_path: identity.project,
      body: "Handle errors consistently",
      draft: false,
      merge_status: "can_be_merged",
      has_conflicts: false,
      additions: 4,
      deletions: 1,
      reviewers: [],
      assignees: [],
      labels: [],
      created_at: timestamp,
      updated_at: timestamp,
    },
    approvals: [],
    discussions,
    pipelines: [],
    has_issues: false,
  };
}

function ReviewConsumer() {
  const review = useMRFeedback(identity.workspaceId, identity.project, identity.iid, identity.host);
  const actions = useMRActions(review.refresh);
  if (!review.feedback) return null;
  return (
    <>
      {review.error && <p role="alert">{review.error}</p>}
      <MRDiscussionsSection
        discussions={review.feedback.discussions}
        mrUrl={mrUrl}
        busy={actions.pendingAction !== null}
        onReply={(discussionId, body) =>
          actions.run("reply", () => createMRDiscussionNote({ ...identity, discussionId, body }))
        }
        onResolve={(discussionId) =>
          actions.run("resolve", () => resolveMRDiscussion({ ...identity, discussionId }))
        }
        onAddContext={() => {}}
      />
    </>
  );
}

function thread(id = "thread-a") {
  return within(screen.getByTestId(`gitlab-discussion-${id}`));
}

function textarea(id = "thread-a") {
  return thread(id).getByRole("textbox", { name: "Discussion reply" }) as HTMLTextAreaElement;
}

function replyButton(id = "thread-a") {
  return thread(id).getByRole("button", { name: "Reply" }) as HTMLButtonElement;
}

function edit(value: string, id = "thread-a") {
  fireEvent.change(textarea(id), { target: { value } });
}

async function mount() {
  await act(async () => {
    render(
      <TooltipProvider>
        <ToastProvider>
          <ReviewConsumer />
        </ToastProvider>
      </TooltipProvider>,
    );
  });
  expect(feedbackReads).toBe(1);
}

async function submit(value = submitted, id = "thread-a") {
  edit(value, id);
  fireEvent.click(replyButton(id));
  await act(async () => {});
  expect(posts.at(-1)?.body).toEqual({
    project: identity.project,
    iid: identity.iid,
    discussion_id: id,
    body: value.trim(),
  });
  expect(replyButton(id).disabled).toBe(true);
  expect(textarea(id).disabled).toBe(false);
}

async function settlePost(index = 0, success = true) {
  const post = posts[index];
  const note: GitLabMRNote = {
    id: 100 + index,
    author: "author",
    body: post.body.body,
    created_at: timestamp,
    updated_at: timestamp,
  };
  if (success) {
    discussions = discussions.map((item) =>
      item.id === post.body.discussion_id ? { ...item, notes: [...item.notes, note] } : item,
    );
  }
  await act(async () => {
    post.response.resolve(success ? json(note) : json({ error: "Reply denied" }, 500));
  });
}

async function settleRefresh(success = true) {
  await act(async () => {
    refreshes
      .at(-1)!
      .resolve(success ? json(feedback()) : json({ error: "Refresh unavailable" }, 500));
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  held = [];
  posts = [];
  refreshes = [];
  feedbackReads = 0;
  holdRefresh = false;
  discussions = [discussion("thread-a"), discussion("thread-b")];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL, options?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname === "/api/v1/gitlab/mrs/discussions/notes") {
      expect(options?.method).toBe("POST");
      expect(url.searchParams.get("workspace_id")).toBe(identity.workspaceId);
      expect(url.searchParams.get("expected_host")).toBe(identity.host);
      const response = deferredResponse();
      posts.push({ body: JSON.parse(String(options?.body)) as PostedReply, response });
      return response.promise;
    }
    if (url.pathname === "/api/v1/gitlab/mrs/feedback") {
      feedbackReads += 1;
      if (holdRefresh && feedbackReads > 1) {
        const response = deferredResponse();
        refreshes.push(response);
        return response.promise;
      }
      return json(feedback());
    }
    if (url.pathname === "/api/v1/gitlab/mrs/files") return json({ files: [] });
    if (url.pathname === "/api/v1/gitlab/mrs/commits") return json({ commits: [] });
    if (url.pathname === "/api/v1/system/logs/frontend-errors")
      return new Response(null, { status: 204 });
    throw new Error(`Unexpected fetch route: ${url.pathname}`);
  });
});

afterEach(async () => {
  while (held.some((response) => !response.settled)) {
    await act(async () => {
      for (const response of held) if (!response.settled) response.resolve(json(feedback()));
    });
  }
  await act(async () => {
    await vi.runOnlyPendingTimersAsync();
  });
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("GitLab discussion reply draft settlement", () => {
  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.11, .12
  it("preserves continued draft after successful post and refresh", async () => {
    await mount();
    await submit();
    edit(continued);
    await settlePost();
    expect(feedbackReads).toBe(2);
    expect(screen.getByText(successNotice)).toBeTruthy();
    expect(thread().getByText(submitted)).toBeTruthy();
    expect(textarea().value).toBe(continued);
  });

  it("clears unchanged successful raw draft", async () => {
    await mount();
    await submit(` \n${submitted}\n  `);
    await settlePost();
    expect(thread().getByText(submitted)).toBeTruthy();
    expect(textarea().value).toBe("");
    expect(replyButton().disabled).toBe(true);
  });

  it("preserves whitespace-only edits to submitted draft", async () => {
    await mount();
    await submit();
    const edited = ` ${submitted}\n`;
    edit(edited);
    await settlePost();
    expect(textarea().value).toBe(edited);
  });

  it("preserves intentional clear while posting", async () => {
    await mount();
    await submit();
    edit("");
    await settlePost();
    expect(textarea().value).toBe("");
    expect(posts).toHaveLength(1);
    expect(replyButton().disabled).toBe(true);
  });

  it("does not submit blank draft", async () => {
    await mount();
    edit(" \n ");
    expect(replyButton().disabled).toBe(true);
    fireEvent.click(replyButton());
    expect(posts).toHaveLength(0);
    expect(textarea().value).toBe(" \n ");
  });

  it("clears draft restored exactly to submitted snapshot", async () => {
    await mount();
    await submit();
    edit(continued);
    edit(submitted);
    await settlePost();
    expect(textarea().value).toBe("");
  });
});

describe("GitLab discussion reply failure and retry", () => {
  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.13
  it("retains unchanged draft after post failure", async () => {
    await mount();
    await submit();
    await settlePost(0, false);
    expect(textarea().value).toBe(submitted);
    expect(screen.getByText("Reply denied")).toBeTruthy();
    expect(feedbackReads).toBe(1);
    expect(replyButton().disabled).toBe(false);
  });

  it("retains edited draft after post failure and retries current text", async () => {
    await mount();
    await submit();
    edit(continued);
    await settlePost(0, false);
    expect(textarea().value).toBe(continued);
    expect(feedbackReads).toBe(1);
    await submit(continued);
    expect(posts).toHaveLength(2);
    await settlePost(1);
    expect(thread().getByText("Second unsent reply with more findings")).toBeTruthy();
    expect(textarea().value).toBe("");
  });

  it("retains intentional clear after post failure", async () => {
    await mount();
    await submit();
    edit("");
    await settlePost(0, false);
    expect(textarea().value).toBe("");
    expect(replyButton().disabled).toBe(true);
    expect(feedbackReads).toBe(1);
  });
});

describe("GitLab discussion reply refresh and isolation", () => {
  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.14
  it("retains continued draft across delayed refresh", async () => {
    await mount();
    holdRefresh = true;
    await submit();
    edit(continued);
    await settlePost();
    expect(refreshes).toHaveLength(1);
    expect(screen.getByText(successNotice)).toBeTruthy();
    expect(textarea().value).toBe(continued);
    expect(replyButton().disabled).toBe(false);
    const later = `${continued}\nEdited during refresh`;
    edit(later);
    await settleRefresh();
    expect(thread().getByText(submitted)).toBeTruthy();
    expect(textarea().value).toBe(later);
  });

  it("retains continued draft when refresh fails", async () => {
    await mount();
    holdRefresh = true;
    await submit();
    edit(continued);
    await settlePost();
    await settleRefresh(false);
    expect(screen.getByRole("alert").textContent).toContain("Refresh unavailable");
    expect(screen.getByText(successNotice)).toBeTruthy();
    expect(textarea().value).toBe(continued);
    expect(replyButton().disabled).toBe(false);
  });

  it("keeps unchanged successful draft cleared when refresh fails", async () => {
    await mount();
    holdRefresh = true;
    await submit();
    await settlePost();
    await settleRefresh(false);
    expect(screen.getByRole("alert").textContent).toContain("Refresh unavailable");
    expect(screen.getByText(successNotice)).toBeTruthy();
    expect(textarea().value).toBe("");
  });

  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.15
  it("keeps other discussion draft through reply settlement and refreshed reorder", async () => {
    await mount();
    await submit();
    edit(continued, "thread-b");
    discussions = [...discussions].reverse();
    await settlePost();
    expect(textarea().value).toBe("");
    expect(textarea("thread-b").value).toBe(continued);
    expect(thread().getByText(submitted)).toBeTruthy();
    expect(thread("thread-b").queryByText(submitted)).toBeNull();
    expect(screen.getAllByRole("article")[0].getAttribute("data-testid")).toBe(
      "gitlab-discussion-thread-b",
    );
  });
});
