import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";
import type { TaskPR } from "@/lib/types/github";

const fetchSessionMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: (...args: unknown[]) => fetchSessionMock(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

import { ReadyToMergeActions, safePullRequestUrl } from "./ready-to-merge-row";
import { QueueGroup } from "./queue-group";

const SEND_BACK = "Send it back";
const ALWAYS_HUMAN = "Merging a pull request is always human.";
const PR_URL = "https://github.com/a/b/pull/1";

beforeEach(() => {
  fetchSessionMock.mockReset().mockResolvedValue({
    session: { id: "s-1", state: "IDLE", queue_incarnation_id: "inc" },
  });
});
afterEach(cleanup);

function task(state = "IDLE", withSession = true): AttentionTask {
  return {
    id: "t-1",
    title: "Task 1",
    identifier: "KAN-1",
    statusSummary: withSession ? { primary_session: { id: "s-1", state } } : undefined,
  } as AttentionTask;
}

function item(t: AttentionTask): QueueItem {
  return { group: "ready_to_merge", id: t.id, task: t, lastActivityAtMs: undefined, ageMs: 0 };
}

function prs(url: string): ReadonlyMap<string, TaskPR[]> {
  return new Map([["t-1", [{ id: "p", task_id: "t-1", state: "open", pr_url: url } as TaskPR]]]);
}

function renderActions(t: AttentionTask, map: ReadonlyMap<string, TaskPR[]>, canManage: boolean) {
  return render(
    <ReadyToMergeActions item={item(t)} prsByTaskId={map} canManage={canManage}>
      {(actions) => <div>{actions}</div>}
    </ReadyToMergeActions>,
  );
}

describe("safePullRequestUrl", () => {
  it("allows http(s) only", () => {
    expect(safePullRequestUrl(PR_URL)).toBe(PR_URL);
    expect(safePullRequestUrl("http://x.test/p")).toBe("http://x.test/p");
    expect(safePullRequestUrl("javascript:alert(1)")).toBeUndefined();
    expect(safePullRequestUrl("data:text/html,x")).toBeUndefined();
    expect(safePullRequestUrl("not a url")).toBeUndefined();
    expect(safePullRequestUrl("")).toBeUndefined();
    expect(safePullRequestUrl(undefined)).toBeUndefined();
  });
});

describe("ReadyToMergeActions", () => {
  it("shows Open the PR as a new-tab link, also for a reader, and no send-back for a reader", () => {
    renderActions(task(), prs(PR_URL), false);
    const link = screen.getByRole("link", { name: "Open the PR" });
    expect(link.getAttribute("href")).toBe(PR_URL);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(screen.queryByRole("button", { name: SEND_BACK })).toBeNull();
  });

  it("omits the link for an unsafe or missing PR url", () => {
    renderActions(task(), prs("javascript:alert(1)"), true);
    expect(screen.queryByRole("link", { name: "Open the PR" })).toBeNull();
    cleanup();
    renderActions(task(), new Map(), true);
    expect(screen.queryByRole("link", { name: "Open the PR" })).toBeNull();
  });

  it("offers Send it back only to a manager with an accepting primary session", () => {
    renderActions(task("FAILED"), new Map(), true);
    expect(screen.queryByRole("button", { name: SEND_BACK })).toBeNull();
    cleanup();
    renderActions(task("IDLE", false), new Map(), true);
    expect(screen.queryByRole("button", { name: SEND_BACK })).toBeNull();
    cleanup();
    renderActions(task(), new Map(), true);
    expect(screen.getByRole("button", { name: SEND_BACK })).not.toBeNull();
  });

  it("opens and closes the inline form and returns focus to the trigger", async () => {
    renderActions(task(), new Map(), true);
    const trigger = screen.getByRole("button", { name: SEND_BACK });
    await act(async () => {
      fireEvent.click(trigger);
    });
    expect(screen.getByRole("textbox")).not.toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      await new Promise((r) => requestAnimationFrame(() => r(null)));
    });
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("does not reopen the form when the session accepts messages again", async () => {
    const view = renderActions(task("IDLE"), new Map(), true);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: SEND_BACK }));
    });
    expect(screen.getByRole("textbox")).not.toBeNull();

    const rerender = (state: string) =>
      view.rerender(
        <ReadyToMergeActions item={item(task(state))} prsByTaskId={new Map()} canManage>
          {(actions) => <div>{actions}</div>}
        </ReadyToMergeActions>,
      );
    await act(async () => rerender("FAILED"));
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("button", { name: SEND_BACK })).toBeNull();

    await act(async () => rerender("IDLE"));
    const trigger = screen.getByRole("button", { name: SEND_BACK });
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });
});

describe("QueueGroup Ready to merge header", () => {
  const props = {
    group: "ready_to_merge" as const,
    items: [item(task())],
    stepNameByTaskId: new Map<string, string>(),
    prsByTaskId: new Map<string, TaskPR[]>(),
  };

  it("shows the always-human line only with phase 2 on", () => {
    render(<QueueGroup {...props} phase2 canManage />);
    expect(screen.getByText(ALWAYS_HUMAN)).not.toBeNull();
    cleanup();
    render(<QueueGroup {...props} />);
    expect(screen.queryByText(ALWAYS_HUMAN)).toBeNull();
    expect(screen.queryByRole("button", { name: SEND_BACK })).toBeNull();
  });

  it("does not show the line on other groups", () => {
    render(<QueueGroup {...props} group="working" phase2 canManage />);
    expect(screen.queryByText(ALWAYS_HUMAN)).toBeNull();
  });
});
