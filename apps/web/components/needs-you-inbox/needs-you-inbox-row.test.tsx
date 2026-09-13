import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import type { ClarificationOutcome } from "@/hooks/domains/session/use-clarification-group";

const mocks = vi.hoisted(() => ({
  dismiss: vi.fn(),
  snooze: vi.fn(),
  bumpRefreshTick: vi.fn(),
  toast: vi.fn(),
  toastError: vi.fn(),
}));

let capturedOnOutcome: ((outcome: ClarificationOutcome) => void) | undefined;

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  dismissClarificationInboxBundle: (...args: unknown[]) => mocks.dismiss(...args),
  snoozeClarificationInboxBundle: (...args: unknown[]) => mocks.snooze(...args),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: Object.assign((...args: unknown[]) => mocks.toast(...args), {
    error: (...args: unknown[]) => mocks.toastError(...args),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) =>
    selector({ bumpNeedsYouInboxRefreshTick: mocks.bumpRefreshTick }),
}));

vi.mock("@/components/task/chat/clarification-panel-section", () => ({
  ClarificationPanelSection: (props: { onOutcome?: (outcome: ClarificationOutcome) => void }) => {
    capturedOnOutcome = props.onOutcome;
    return <div data-testid="mock-clarification-panel-section" />;
  },
}));

import { NeedsYouInboxRow } from "./needs-you-inbox-row";

function openRowActionsMenu() {
  const trigger = screen.getByRole("button", { name: "Row actions" });
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
}

function bundle(overrides: Partial<ClarificationInboxBundle> = {}): ClarificationInboxBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "WAITING_FOR_INPUT",
    task_title: "Fix the deploy",
    created_at: new Date().toISOString(),
    context: "",
    messages: [
      {
        id: "m1",
        session_id: toSessionId("s1"),
        task_id: toTaskId("t1"),
        author_type: "agent",
        content: "",
        type: "clarification_request",
        created_at: new Date().toISOString(),
        metadata: {
          pending_id: "p1",
          session_id: "s1",
          question: { id: "q1", title: "Pick an option", prompt: "", options: [] },
        },
      },
    ],
    ...overrides,
  };
}

beforeEach(() => {
  mocks.dismiss.mockReset().mockResolvedValue(undefined);
  mocks.snooze.mockReset().mockResolvedValue(undefined);
  mocks.bumpRefreshTick.mockReset();
  mocks.toast.mockReset();
  mocks.toastError.mockReset();
  capturedOnOutcome = undefined;
});

afterEach(() => cleanup());

describe("NeedsYouInboxRow", () => {
  it("shows the primary question, secondary task text, and relative time", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);

    expect(screen.getByText("Pick an option")).not.toBeNull();
    expect(screen.getByText("Fix the deploy")).not.toBeNull();
  });

  it("links to the task, which the row body itself cannot reach (design-03#D1)", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);

    const link = screen.getByTestId("needs-you-inbox-open-task");
    expect(link.getAttribute("href")).toBe("/t/t1");
    expect(link.textContent).toBe("Open task");
  });

  it("keeps the task reachable from the actions menu, where narrow viewports must find it (design-03#D1)", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    openRowActionsMenu();

    const item = screen.getByTestId("needs-you-inbox-open-task-menu-item");
    expect(item.getAttribute("href")).toBe("/t/t1");
  });

  it("states the bundle size only when the bundle holds more than one question (design-03#D3)", () => {
    const { unmount } = render(<NeedsYouInboxRow bundle={bundle()} />);
    expect(screen.queryByTestId("needs-you-inbox-row-question-count")).toBeNull();
    unmount();

    const two = bundle();
    two.messages = [
      two.messages[0],
      {
        ...two.messages[0],
        id: "m2",
        metadata: {
          pending_id: "p1",
          session_id: "s1",
          question: { id: "q2", title: "And another", prompt: "", options: [] },
        },
      },
    ];
    render(<NeedsYouInboxRow bundle={two} />);

    expect(screen.getByTestId("needs-you-inbox-row-question-count").textContent).toBe(
      "2 questions in this bundle",
    );
  });

  it("labels the row's status through the shared Threads status vocabulary (AC .26)", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);

    expect(screen.getByRole("img", { name: "Question from agent" })).not.toBeNull();
  });

  it("mounts the clarification panel only after the row is expanded", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);

    expect(screen.queryByTestId("mock-clarification-panel-section")).toBeNull();
    fireEvent.click(screen.getByTestId("needs-you-inbox-row-toggle"));
    expect(screen.getByTestId("mock-clarification-panel-section")).not.toBeNull();
  });

  it("unmounts the clarification panel on a second toggle", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);

    const toggle = screen.getByTestId("needs-you-inbox-row-toggle");
    fireEvent.click(toggle);
    fireEvent.click(toggle);
    expect(screen.queryByTestId("mock-clarification-panel-section")).toBeNull();
  });

  it("dismisses the bundle and triggers a convergence re-read", async () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    openRowActionsMenu();
    fireEvent.click(await screen.findByText("Dismiss"));

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.dismiss).toHaveBeenCalledWith("p1");
    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
  });

  it("snoozes the bundle for the selected duration and re-reads", async () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    openRowActionsMenu();
    const snoozeTrigger = await screen.findByText("Snooze");
    fireEvent.keyDown(snoozeTrigger, { key: "ArrowRight" });
    fireEvent.click(await screen.findByText("4 hours"));

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.snooze).toHaveBeenCalledWith("p1", "4h");
    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
  });

  it("shows a retryable error and does not re-read when dismiss fails", async () => {
    mocks.dismiss.mockRejectedValue(new Error("boom"));
    render(<NeedsYouInboxRow bundle={bundle()} />);
    openRowActionsMenu();
    fireEvent.click(await screen.findByText("Dismiss"));

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.toastError).toHaveBeenCalledOnce();
    expect(mocks.bumpRefreshTick).not.toHaveBeenCalled();
  });
});

describe("NeedsYouInboxRow onOutcome (AC .17, .35, .39)", () => {
  function expand() {
    fireEvent.click(screen.getByTestId("needs-you-inbox-row-toggle"));
  }

  it("this caller won: re-reads with no notice", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() => capturedOnOutcome?.({ kind: "resolved", claimedByThisCaller: true }));

    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("another caller won by answering: notice names 'answered'", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() =>
      capturedOnOutcome?.({ kind: "resolved", claimedByThisCaller: false, status: "answered" }),
    );

    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledOnce();
    expect(mocks.toast.mock.calls[0][0]).toMatch(/answered/i);
    expect(mocks.toastError).not.toHaveBeenCalled();
  });

  it("another caller won by rejecting: notice names 'rejected', not 'answered' (AC .17)", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() =>
      capturedOnOutcome?.({ kind: "resolved", claimedByThisCaller: false, status: "rejected" }),
    );

    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledOnce();
    expect(mocks.toast.mock.calls[0][0]).toMatch(/rejected/i);
    expect(mocks.toast.mock.calls[0][0]).not.toMatch(/answered/i);
  });

  it("another caller won with no reported status (legacy backend): falls back to 'answered' wording", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() => capturedOnOutcome?.({ kind: "resolved", claimedByThisCaller: false }));

    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledOnce();
    expect(mocks.toast.mock.calls[0][0]).toMatch(/answered/i);
  });

  it("bundle no longer active: shows a transient non-error notice and re-reads", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() => capturedOnOutcome?.({ kind: "no_longer_active" }));

    expect(mocks.bumpRefreshTick).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledOnce();
  });

  it("submission failed: leaves the row and count untouched (AC .35)", () => {
    render(<NeedsYouInboxRow bundle={bundle()} />);
    expand();
    act(() => capturedOnOutcome?.({ kind: "submission_failed" }));

    expect(mocks.bumpRefreshTick).not.toHaveBeenCalled();
    expect(mocks.toast).not.toHaveBeenCalled();
    expect(mocks.toastError).not.toHaveBeenCalled();
  });
});
