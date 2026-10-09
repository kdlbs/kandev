// Coarse-pointer tablet coverage: apps/web/AGENTS.md ("Responsive and touch
// surfaces") defines tablet as a coarse-pointer fallback between md and lg,
// so a tablet viewport (isMobile false, isFinePointer false) must get the
// same compact/touch-target action treatment as a phone.
import { cleanup, render, screen } from "@testing-library/react";
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

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: false }),
}));

import { InboxHistoryRow } from "./inbox-history-row";

const TASK_ID = "t1";
const SESSION_ID = "s1";

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

beforeEach(() => {
  mocks.copyToClipboard.mockReset().mockResolvedValue(true);
  mocks.toast.mockReset();
  mocks.toastError.mockReset();
});

afterEach(() => cleanup());

describe("InboxHistoryRow — coarse-pointer tablet actions", () => {
  it("renders icon-only actions with an aria-label, not the desktop text-labelled pair", () => {
    render(<InboxHistoryRow bundle={bundle()} />);
    expect(screen.getByTestId("inbox-history-open-task").getAttribute("aria-label")).toBe(
      "Open task",
    );
    expect(screen.getByTestId("inbox-history-copy-id").getAttribute("aria-label")).toBe(
      "Copy identifier",
    );
  });

  it("sizes both actions to the 44px touch-target minimum", () => {
    render(<InboxHistoryRow bundle={bundle()} />);
    const openTask = screen.getByTestId("inbox-history-open-task");
    const copyId = screen.getByTestId("inbox-history-copy-id");
    expect(openTask.className).toContain("min-h-11");
    expect(openTask.className).toContain("min-w-11");
    expect(copyId.className).toContain("min-h-11");
    expect(copyId.className).toContain("min-w-11");
  });
});
