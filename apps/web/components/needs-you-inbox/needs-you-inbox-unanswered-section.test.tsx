import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxUnansweredBundle } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  listUnanswered: vi.fn(),
  close: vi.fn(),
  toastError: vi.fn(),
  lateAnswerTargets: [] as Array<string | null | undefined>,
  lateAnswerSend: vi.fn(),
  state: {
    workspaces: {
      activeId: "w1" as string | null,
      items: [{ id: "w1", clarification_unanswered_inbox_enabled: true }],
    },
    needsYouInbox: { byWorkspaceId: {} as Record<string, unknown> },
  },
}));

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listUnansweredClarificationInbox: (...args: unknown[]) => mocks.listUnanswered(...args),
  closeUnansweredClarificationInboxBundle: (...args: unknown[]) => mocks.close(...args),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: Object.assign(vi.fn(), { error: (...args: unknown[]) => mocks.toastError(...args) }),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) => selector(mocks.state),
}));

vi.mock("@/hooks/use-late-clarification-message", () => ({
  useLateClarificationMessage: (
    _message: unknown,
    options?: { targetSessionId?: string | null },
  ) => {
    mocks.lateAnswerTargets.push(options?.targetSessionId);
    return { state: { status: "idle", snapshot: null }, send: mocks.lateAnswerSend };
  },
}));

vi.mock("@/components/task/chat/clarification-input-overlay", () => ({
  ClarificationInputOverlay: ({ mode }: { mode?: string }) => (
    <div data-testid="late-answer-form" data-mode={mode} />
  ),
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));

import { NeedsYouInboxUnansweredSection } from "./needs-you-inbox-unanswered-section";
import { resolveUnansweredTargetSessionId } from "./needs-you-inbox-unanswered-row";

function unansweredBundle(
  overrides: Partial<ClarificationInboxUnansweredBundle> = {},
): ClarificationInboxUnansweredBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s-old",
    session_state: "COMPLETED",
    task_title: "Migrate billing",
    created_at: "2026-10-01T09:00:00Z",
    context: "",
    messages: [
      {
        id: "m1",
        session_id: "s-old",
        task_id: "t1",
        content: "Which database?",
        metadata: { pending_id: "p1", question: { id: "q1", prompt: "Which database?" } },
      } as unknown as ClarificationInboxUnansweredBundle["messages"][number],
    ],
    original_session_terminal: true,
    send_to_task_session_id: "s-primary",
    ...overrides,
  };
}

function page(bundles: ClarificationInboxUnansweredBundle[], enabled = true) {
  return { enabled, retention_days: 14, bundles, count: bundles.length };
}

beforeEach(() => {
  mocks.listUnanswered.mockReset();
  mocks.close.mockReset().mockResolvedValue(undefined);
  mocks.toastError.mockReset();
  mocks.lateAnswerSend.mockReset();
  mocks.lateAnswerTargets = [];
  mocks.state.workspaces.activeId = "w1";
  mocks.state.workspaces.items = [{ id: "w1", clarification_unanswered_inbox_enabled: true }];
});

afterEach(() => cleanup());

describe("NeedsYouInboxUnansweredSection", () => {
  it("renders nothing and never fetches while the workspace setting is off", () => {
    mocks.state.workspaces.items = [{ id: "w1", clarification_unanswered_inbox_enabled: false }];
    const { container } = render(<NeedsYouInboxUnansweredSection />);
    expect(container.innerHTML).toBe("");
    expect(mocks.listUnanswered).not.toHaveBeenCalled();
  });

  it("lists unanswered questions for the active workspace", async () => {
    mocks.listUnanswered.mockResolvedValue(page([unansweredBundle()]));
    render(<NeedsYouInboxUnansweredSection />);
    expect(await screen.findByTestId("needs-you-unanswered-row")).not.toBeNull();
    expect(screen.getByText("Unanswered (agent not waiting)")).not.toBeNull();
    expect(screen.getByText("Which database?")).not.toBeNull();
    expect(mocks.listUnanswered).toHaveBeenCalledWith("w1", { cache: "no-store" });
  });

  it("hides the section when the server reports nothing unanswered", async () => {
    mocks.listUnanswered.mockResolvedValue(page([]));
    render(<NeedsYouInboxUnansweredSection />);
    await waitFor(() => expect(mocks.listUnanswered).toHaveBeenCalled());
    expect(screen.queryByTestId("needs-you-unanswered-section")).toBeNull();
  });

  it("closes a question and reloads the section", async () => {
    mocks.listUnanswered
      .mockResolvedValueOnce(page([unansweredBundle()]))
      .mockResolvedValueOnce(page([]));
    render(<NeedsYouInboxUnansweredSection />);
    fireEvent.click(await screen.findByTestId("needs-you-unanswered-close"));
    await waitFor(() => expect(mocks.close).toHaveBeenCalledWith("p1"));
    await waitFor(() => expect(screen.queryByTestId("needs-you-unanswered-row")).toBeNull());
    expect(mocks.listUnanswered).toHaveBeenCalledTimes(2);
  });

  it("defaults the late answer to the original session and switches only on an explicit choice", async () => {
    mocks.listUnanswered.mockResolvedValue(page([unansweredBundle()]));
    render(<NeedsYouInboxUnansweredSection />);
    fireEvent.click(await screen.findByTestId("needs-you-unanswered-row-toggle"));

    expect(screen.getByTestId("late-answer-form").getAttribute("data-mode")).toBe("late");
    expect(mocks.lateAnswerTargets.at(-1)).toBe("s-old");
    fireEvent.click(screen.getByTestId("needs-you-unanswered-target-task"));
    expect(mocks.lateAnswerTargets.at(-1)).toBe("s-primary");
    fireEvent.click(screen.getByTestId("needs-you-unanswered-target-original"));
    expect(mocks.lateAnswerTargets.at(-1)).toBe("s-old");
  });

  it("offers no target choice while the asking session is still live", async () => {
    mocks.listUnanswered.mockResolvedValue(
      page([unansweredBundle({ original_session_terminal: false, send_to_task_session_id: null })]),
    );
    render(<NeedsYouInboxUnansweredSection />);
    fireEvent.click(await screen.findByTestId("needs-you-unanswered-row-toggle"));
    expect(screen.queryByTestId("needs-you-unanswered-target")).toBeNull();
  });
});

describe("resolveUnansweredTargetSessionId", () => {
  it("falls back to the asking session when no task session is offered", () => {
    const bundle = unansweredBundle({ send_to_task_session_id: null });
    expect(resolveUnansweredTargetSessionId(bundle, "task")).toBe("s-old");
    expect(resolveUnansweredTargetSessionId(unansweredBundle(), "task")).toBe("s-primary");
    expect(resolveUnansweredTargetSessionId(unansweredBundle(), "original")).toBe("s-old");
  });
});
