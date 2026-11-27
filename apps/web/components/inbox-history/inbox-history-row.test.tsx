import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { Message } from "@/lib/types/http";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  toast: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/lib/utils/copy-to-clipboard", () => ({
  copyToClipboard: (...args: unknown[]) => mocks.copyToClipboard(...args),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: Object.assign((...args: unknown[]) => mocks.toast(...args), {
    error: (...args: unknown[]) => mocks.toastError(...args),
  }),
}));

import { InboxHistoryRow } from "./inbox-history-row";

const TASK_ID = "t1";
const SESSION_ID = "s1";
const TOGGLE_TESTID = "inbox-history-row-toggle";
const DETAIL_TESTID = "inbox-history-clarification-detail";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    author_type: "agent",
    content: "",
    type: "clarification_request",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

function bundle(overrides: Partial<InboxHistoryBundle> = {}): InboxHistoryBundle {
  return {
    pending_id: "p1",
    task_id: TASK_ID,
    session_id: SESSION_ID,
    session_state: "COMPLETED",
    task_title: "Fix the deploy",
    created_at: new Date().toISOString(),
    context: "shared context",
    messages: [
      message({
        metadata: {
          question: {
            id: "q1",
            title: "Pick a branch",
            prompt: "Which branch should I use?",
            options: [{ option_id: "o1", label: "main", description: "" }],
          },
        },
      }),
    ],
    kind: "clarification",
    reason: "session_ended",
    asking_turn_id: "turn-1",
    ...overrides,
  };
}

function permissionBundle(overrides: Partial<InboxHistoryBundle> = {}): InboxHistoryBundle {
  return bundle({
    kind: "permission",
    messages: [
      message({
        content: "Allow deleting tmp/?",
        type: "permission_request",
        metadata: { options: [{ kind: "allow", name: "Allow", option_id: "o1" }] },
      }),
    ],
    ...overrides,
  });
}

function expandRow() {
  fireEvent.click(screen.getByTestId(TOGGLE_TESTID));
}

function markdownBundle(title = "Choose `storage`"): InboxHistoryBundle {
  return bundle({
    context: "Keep `context` literal.",
    messages: [
      message({
        metadata: {
          question: {
            id: "markdown",
            title,
            prompt:
              "Read **guidance** in the [storage guide](https://example.com/storage).\n\n1. Check retention\n2. Compare costs",
            options: [{ option_id: "postgres", label: "`Postgres`", description: "" }],
          },
        },
      }),
    ],
  });
}

beforeEach(() => {
  mocks.copyToClipboard.mockReset().mockResolvedValue(true);
  mocks.toast.mockReset();
  mocks.toastError.mockReset();
});

afterEach(() => cleanup());

describe("InboxHistoryRow — kind and content", () => {
  it("renders a clarification icon, not the permission icon, for a clarification bundle", () => {
    render(<InboxHistoryRow bundle={bundle({ kind: "clarification" })} />);
    expect(screen.queryByTestId("inbox-history-permission-icon")).toBeNull();
  });

  it("labels a permission bundle with the shipped IconShieldQuestion/text-amber-500 pairing (AC .12)", () => {
    render(<InboxHistoryRow bundle={permissionBundle()} />);
    const icon = screen.getByTestId("inbox-history-permission-icon");
    expect(icon).not.toBeNull();
    expect(icon.querySelector("svg")?.getAttribute("class")).toContain("text-amber-500");
  });

  it("shows the reason badge, not a raw pending status (AC .9)", () => {
    render(<InboxHistoryRow bundle={bundle({ reason: "unreadable" })} />);
    expect(screen.getByTestId("inbox-history-row-reason").textContent).toMatch(
      /could not be read/i,
    );
  });

  it("reveals every question's title, prompt and option labels on expand (AC .30, AC .13)", () => {
    render(<InboxHistoryRow bundle={bundle()} />);
    expandRow();

    const detail = screen.getByTestId(DETAIL_TESTID);
    expect(detail.textContent).toContain("Pick a branch");
    expect(detail.textContent).toContain("Which branch should I use?");
    expect(detail.textContent).toContain("main");
    expect(detail.textContent).toContain("shared context");
  });

  it("renders permission content and options[].name, not a question object (AC .30)", () => {
    render(<InboxHistoryRow bundle={permissionBundle()} />);
    expandRow();

    const detail = screen.getByTestId("inbox-history-permission-detail");
    expect(detail.textContent).toContain("Allow deleting tmp/?");
    expect(detail.textContent).toContain("Allow");
  });
});

describe("InboxHistoryRow — turn identity and step label", () => {
  it("shows the step-starts-no-agent label only when the field is true (AC .11)", () => {
    render(<InboxHistoryRow bundle={bundle({ step_starts_no_agent: true })} />);
    expandRow();
    expect(screen.getByTestId("inbox-history-step-starts-no-agent")).not.toBeNull();
  });

  it("omits the step-starts-no-agent label when the field is absent (unknown, not guessed)", () => {
    render(<InboxHistoryRow bundle={bundle({ step_starts_no_agent: undefined })} />);
    expandRow();
    expect(screen.queryByTestId("inbox-history-step-starts-no-agent")).toBeNull();
  });

  it("identifies the superseding turn only when the reason is superseded (AC .10)", () => {
    render(
      <InboxHistoryRow
        bundle={bundle({ reason: "superseded", asking_turn_id: "t1", superseding_turn_id: "t2" })}
      />,
    );
    expandRow();
    const turnIdentity = screen.getByTestId("inbox-history-turn-identity");
    expect(turnIdentity.textContent).toContain("t1");
    expect(turnIdentity.textContent).toContain("t2");
  });

  it("omits the superseding turn for a same-turn permission supersession (AC .10)", () => {
    render(
      <InboxHistoryRow
        bundle={bundle({
          reason: "superseded",
          asking_turn_id: "t1",
          superseding_turn_id: undefined,
        })}
      />,
    );
    expandRow();
    const turnIdentity = screen.getByTestId("inbox-history-turn-identity");
    expect(turnIdentity.textContent).toContain("t1");
    expect(turnIdentity.textContent).not.toContain("undefined");
  });
});

// @covers AC-UI-CLARIFICATION-MARKDOWN-001.1 through .4, .6, .7
describe("InboxHistoryRow Markdown", () => {
  it("formats clarification summaries and expanded questions", () => {
    const value = markdownBundle();
    const original = JSON.stringify(value);
    render(<InboxHistoryRow bundle={value} />);

    expect(screen.getByTestId(TOGGLE_TESTID).querySelector("code")?.textContent).toBe("storage");
    expandRow();
    const detail = screen.getByTestId(DETAIL_TESTID);
    expect(Array.from(detail.querySelectorAll("code"), (node) => node.textContent)).toEqual([
      "storage",
      "Postgres",
    ]);
    expect(detail.querySelector("strong")?.textContent).toBe("guidance");
    expect(detail.querySelectorAll("ol > li")).toHaveLength(2);
    expect(detail.textContent).toContain("Keep `context` literal.");
    expect(JSON.stringify(value)).toBe(original);
  });

  it("keeps summary links passive and detail links interactive", () => {
    render(
      <InboxHistoryRow bundle={markdownBundle("Read [guide](https://example.com/storage)")} />,
    );
    const toggle = screen.getByTestId(TOGGLE_TESTID);
    expect(toggle.querySelector("a")).toBeNull();
    expect(toggle.textContent).toContain("Read guide");

    expandRow();
    const detail = screen.getByTestId(DETAIL_TESTID);
    const link = detail.querySelector<HTMLAnchorElement>('a[href="https://example.com/storage"]');
    expect(link).not.toBeNull();
    expect(link?.target).toBe("_blank");
    expect(link?.rel).toBe("noopener noreferrer");
    fireEvent.click(link!);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(toggle);
    expect(screen.queryByTestId(DETAIL_TESTID)).toBeNull();
  });

  it("renders title-less prompts as full question content", () => {
    render(<InboxHistoryRow bundle={markdownBundle("")} />);
    expect(screen.getByTestId(TOGGLE_TESTID).querySelector("strong")?.textContent).toBe("guidance");
    expandRow();
    const detail = screen.getByTestId(DETAIL_TESTID);
    expect(detail.querySelectorAll("ol > li")).toHaveLength(2);
    expect(detail.querySelector('a[href="https://example.com/storage"]')).not.toBeNull();
  });

  it("keeps context-only summaries and permission content literal", () => {
    const value = bundle({ messages: [], context: "Keep **context** literal" });
    const { rerender } = render(<InboxHistoryRow bundle={value} />);
    const toggle = screen.getByTestId(TOGGLE_TESTID);
    expect(toggle.textContent).toContain("Keep **context** literal");
    expect(toggle.querySelector("strong")).toBeNull();

    rerender(
      <InboxHistoryRow
        bundle={permissionBundle({
          messages: [message({ content: "Allow **deletion**?", type: "permission_request" })],
        })}
      />,
    );
    expandRow();
    const detail = screen.getByTestId("inbox-history-permission-detail");
    expect(detail.textContent).toBe("Allow **deletion**?");
    expect(detail.querySelector("strong")).toBeNull();
  });
});

describe("InboxHistoryRow — actions", () => {
  it("exposes only open-task and copy-id actions, no answer/dismiss/snooze affordance (AC .14)", () => {
    render(<InboxHistoryRow bundle={bundle()} />);
    expect(screen.getByTestId("inbox-history-open-task")).not.toBeNull();
    expect(screen.getByTestId("inbox-history-copy-id")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /dismiss/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /snooze/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /^answer/i })).toBeNull();
  });

  it("copies the bundle's pending_id when the copy action is used", async () => {
    render(<InboxHistoryRow bundle={bundle({ pending_id: "bundle-42" })} />);
    fireEvent.click(screen.getByTestId("inbox-history-copy-id"));
    await Promise.resolve();
    expect(mocks.copyToClipboard).toHaveBeenCalledWith("bundle-42");
  });

  it("links to the owning task with the session id", () => {
    render(<InboxHistoryRow bundle={bundle({ task_id: "t-9", session_id: "s-9" })} />);
    expect(screen.getByTestId("inbox-history-open-task").getAttribute("href")).toBe(
      "/t/t-9?sessionId=s-9",
    );
  });
});
