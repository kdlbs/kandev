import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapeGuardEntry,
} from "@/hooks/use-clarification-escape-guard";
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

function fakeEscape(target: EventTarget): KeyboardEvent {
  return {
    key: "Escape",
    target,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
  } as unknown as KeyboardEvent;
}

function renderOverlayWithGuard(messages: Message[]) {
  const scopeRef = createRef<HTMLDivElement>();
  const outsideRef = createRef<HTMLButtonElement>();
  const composerRef = createRef<HTMLInputElement>();
  const onDismiss = vi.fn();
  // A holder object, not a reassigned `let`, so TS doesn't narrow the read
  // in getGuard() to the initializer's type across the closure boundary.
  const holder: { entry: ClarificationEscapeGuardEntry } = { entry: null };
  render(
    <ClarificationEscapeGuardProvider
      value={(entry) => {
        holder.entry = entry;
      }}
    >
      {/* Stands in for the Quick Chat tab bar / resize handles: rendered
          inside the dialog but outside the clarification's shortcut scope. */}
      <button ref={outsideRef} type="button">
        outside
      </button>
      <div ref={scopeRef} tabIndex={-1}>
        {/* Stands in for the message composer: an editable control inside
            the shortcut scope, which is where focus ordinarily sits right
            after sending the message that triggered the clarification. */}
        <input ref={composerRef} />
        <ClarificationInputOverlay
          messages={messages}
          onResolved={vi.fn()}
          onDismiss={onDismiss}
          shortcutScopeRef={scopeRef}
        />
      </div>
    </ClarificationEscapeGuardProvider>,
  );
  return { scopeRef, outsideRef, composerRef, onDismiss, getGuard: () => holder.entry };
}

describe("ClarificationInputOverlay — Escape guard predicate (F1 regression)", () => {
  it("handles Escape while focus is in the composer (editable, in-scope) -- the ordinary post-send state", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, onDismiss, getGuard } = renderOverlayWithGuard(messages);

    expect(getGuard()?.test(fakeEscape(composerRef.current!))).toBe(true);

    fireEvent.keyDown(composerRef.current!, { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("does not claim Escape when the target is outside the shortcut scope (e.g. the tab bar)", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { outsideRef, onDismiss, getGuard } = renderOverlayWithGuard(messages);

    expect(getGuard()?.test(fakeEscape(outsideRef.current!))).toBe(false);

    fireEvent.keyDown(outsideRef.current!, { key: "Escape" });
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("does not claim Escape while submitting", () => {
    // Never resolves, so submitState stays "submitting" for the rest of the
    // test instead of racing a real fetch's microtask resolution.
    fetchMock.mockImplementationOnce(() => new Promise(() => {}));
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, getGuard } = renderOverlayWithGuard(messages);

    fireEvent.click(screen.getByTestId("clarification-option"));

    expect(getGuard()?.test(fakeEscape(composerRef.current!))).toBe(false);
  });

  it("does not claim Escape held with a modifier", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, getGuard } = renderOverlayWithGuard(messages);

    const modified = { ...fakeEscape(composerRef.current!), metaKey: true } as KeyboardEvent;
    expect(getGuard()?.test(modified)).toBe(false);
  });
});

describe("ClarificationInputOverlay — Escape defaultPrevented guard (F3/F4 regression)", () => {
  it("does not collapse the panel when another in-scope consumer already claimed the Escape", () => {
    // Stands in for queued-ghost-message's own Escape handler (cancel edit)
    // or tiptap-suggestion's mention/slash popup close: an in-scope listener
    // between the target and window that claims the key first. Attached on
    // scopeRef itself so it fires during the same bubble dispatch before
    // CarouselKeyboardShortcuts's window listener ever sees the event.
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { onDismiss, scopeRef } = renderOverlay(messages);
    scopeRef.current!.addEventListener("keydown", (e) => e.preventDefault());

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("still collapses the panel on a plain, unclaimed Escape (no regression from the defaultPrevented check)", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { onDismiss, scopeRef } = renderOverlay(messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("guard predicate returns false for an Escape whose defaultPrevented is already true", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, getGuard } = renderOverlayWithGuard(messages);

    const alreadyClaimed = {
      ...fakeEscape(composerRef.current!),
      defaultPrevented: true,
    } as KeyboardEvent;

    expect(getGuard()?.test(alreadyClaimed)).toBe(false);
  });

  it("does not arm the guard when the active message has no resolvable question meta (F4)", () => {
    const badMessage: Message = {
      id: "m-bad",
      session_id: toSessionId("s1"),
      task_id: toTaskId("t1"),
      author_type: "agent",
      content: "Q",
      type: "clarification_request",
      created_at: "2026-05-04T00:00:00Z",
      metadata: { pending_id: "p1" },
    };
    const { composerRef, getGuard } = renderOverlayWithGuard([badMessage]);

    expect(getGuard()?.test(fakeEscape(composerRef.current!))).toBe(false);
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
