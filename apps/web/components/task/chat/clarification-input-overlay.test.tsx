import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ClarificationInputOverlay } from "./clarification-input-overlay";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

const mockUpdateMessage = vi.fn();
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ updateMessage: mockUpdateMessage }),
  }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const fetchMock = vi.fn();

function clarMessage(opts: {
  id: string;
  questionId: string;
  index: number;
  total: number;
}): Message {
  return {
    id: opts.id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-05-04T00:00:00Z",
    metadata: {
      pending_id: "p1",
      question_id: opts.questionId,
      question_index: opts.index,
      question_total: opts.total,
      status: "pending",
      question: {
        id: opts.questionId,
        title: "",
        prompt: `Question ${opts.index + 1}?`,
        options: [{ option_id: "o1", label: "Option 1", description: "" }],
      },
    },
  };
}

function renderOverlay(
  messages: Message[],
  overrides: Partial<{ onResolved: () => void; onDismiss: () => void }> = {},
) {
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = overrides.onResolved ?? vi.fn();
  const onDismiss = overrides.onDismiss ?? vi.fn();
  const utils = render(
    <div ref={scopeRef} tabIndex={-1}>
      <ClarificationInputOverlay
        messages={messages}
        onResolved={onResolved}
        onDismiss={onDismiss}
        shortcutScopeRef={scopeRef}
      />
    </div>,
  );
  return { ...utils, scopeRef, onResolved, onDismiss };
}

beforeEach(() => {
  fetchMock.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
});

afterEach(() => {
  cleanup();
});

describe("ClarificationInputOverlay — Escape key", () => {
  it("dismisses locally without posting a rejection, leaving the question pending", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { onDismiss, scopeRef } = renderOverlay(messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("does not call skipAll (no store update to rejected) on Escape", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { scopeRef } = renderOverlay(messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(mockUpdateMessage).not.toHaveBeenCalled();
  });

  it("still calls onDismiss from any step of a multi-question carousel", () => {
    const messages = [
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m2", questionId: "q2", index: 1, total: 2 }),
    ];
    const { onDismiss, scopeRef } = renderOverlay(messages);

    fireEvent.click(screen.getAllByTestId("clarification-step")[1]);
    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

describe("ClarificationInputOverlay — labelled Skip button", () => {
  it("still POSTs a rejection when the Skip button is clicked", async () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    renderOverlay(messages);

    fireEvent.click(screen.getByTestId("clarification-skip"));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      rejected: true,
      reject_reason: "User skipped",
    });
  });
});
