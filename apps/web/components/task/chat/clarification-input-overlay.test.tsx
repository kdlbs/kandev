import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent, screen } from "@testing-library/react";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationRequestMetadata,
  type Message,
} from "@/lib/types/http";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapeGuardEntry,
  type ClarificationEscapeGuardRegistry,
} from "@/hooks/use-clarification-escape-guard";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import { ClarificationDraftProvider } from "./clarification-draft-context";
import {
  answerDraftIdentity,
  clearAnswerDrafts,
  readAnswerDraft,
} from "@/lib/clarification/answer-draft-cache";
import type { ClarificationOutcome } from "@/hooks/domains/session/use-clarification-group";

/* eslint-disable max-lines -- overlay lifecycle regressions share one deterministic harness. */

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

const mockUpdateMessage = vi.fn();
const mockStore = {
  getState: () => ({ updateMessage: mockUpdateMessage }),
  subscribe: () => () => {},
};
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => mockStore,
  useOptionalAppStoreApi: () => mockStore,
  useOptionalAppStore: (_selector: unknown, fallback: unknown) => fallback,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const fetchMock = vi.fn();
const TESTID_OPTION = "clarification-option";
const TESTID_STEP = "clarification-step";
const TESTID_SKIP = "clarification-skip";
const TESTID_INPUT = "clarification-input";
const TESTID_SIZE_TOGGLE = "clarification-panel-size-toggle";
const TESTID_SUBMIT_ERROR = "clarification-submit-error";
const TESTID_RETRY = "clarification-retry";
const DATA_SELECTED = "data-selected";

function clarMessage(opts: {
  id: string;
  questionId: string;
  index: number;
  total: number;
  pendingId?: string;
  attachments?: Array<Record<string, unknown>>;
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
      pending_id: opts.pendingId ?? "p1",
      question_id: opts.questionId,
      question_index: opts.index,
      question_total: opts.total,
      status: "pending",
      ...(opts.attachments ? { attachments: opts.attachments } : {}),
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
  overrides: Partial<{
    onResolved: () => void;
    onDismiss: () => void;
    onOutcome: (outcome: ClarificationOutcome) => void;
    mode: "active" | "late";
    initialAnswers: readonly {
      question_id: string;
      selected_options?: string[];
      custom_text?: string;
    }[];
    onLateAnswer: (snapshot: {
      messages: readonly Message[];
      answers: readonly {
        question_id: string;
        selected_options?: string[];
        custom_text?: string;
      }[];
    }) => Promise<"sent" | "queued">;
    onToggleSize: () => void;
    sizeExpanded: boolean;
    canExpandPanel: boolean;
  }> = {},
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
        onOutcome={overrides.onOutcome}
        mode={overrides.mode}
        onLateAnswer={overrides.onLateAnswer}
        initialAnswers={overrides.initialAnswers}
        onToggleSize={overrides.onToggleSize}
        sizeExpanded={overrides.sizeExpanded}
        canExpandPanel={overrides.canExpandPanel}
        shortcutScopeRef={scopeRef}
      />
    </div>,
  );
  return { ...utils, scopeRef, onResolved, onDismiss };
}

beforeEach(() => {
  fetchMock.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
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

    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[1]);
    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

describe("ClarificationInputOverlay — native question controls", () => {
  it("hides custom text when the provider does not allow another answer", () => {
    const message = clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 });
    const metadata = message.metadata as ClarificationRequestMetadata;
    metadata.question.allow_custom_text = false;

    renderOverlay([message]);

    expect(screen.getByTestId(TESTID_OPTION)).toBeTruthy();
    expect(screen.queryByTestId("clarification-custom-input")).toBeNull();
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
  // ClarificationInputOverlay is the only registrant in this test tree, so a
  // single-slot holder still faithfully mirrors what it registered.
  const holder: { entry: ClarificationEscapeGuardEntry } = { entry: null };
  const registry: ClarificationEscapeGuardRegistry = {
    register: (_id, predicate) => {
      holder.entry = { test: predicate };
    },
    unregister: () => {
      holder.entry = null;
    },
  };
  render(
    <ClarificationEscapeGuardProvider value={registry}>
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

  it("does not claim Escape while submitting", async () => {
    // Never resolves, so submitState stays "submitting" for the rest of the
    // test instead of racing a real fetch's microtask resolution.
    fetchMock.mockImplementationOnce(() => new Promise(() => {}));
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, getGuard } = renderOverlayWithGuard(messages);

    fireEvent.click(screen.getByTestId("clarification-option"));
    await submitSingleOverlayAnswer();

    expect(getGuard()?.test(fakeEscape(composerRef.current!))).toBe(false);
  });

  it("does not claim Escape after an inactive response leaves retained props mounted", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ code: "not_active" }), { status: 409 }),
    );
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, onDismiss, getGuard } = renderOverlayWithGuard(messages);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();
    await vi.waitFor(() => screen.getByTestId("clarification-expired"));

    expect(getGuard()?.test(fakeEscape(composerRef.current!))).not.toBe(true);
    fireEvent.keyDown(composerRef.current!, { key: "Escape" });
    expect(onDismiss).not.toHaveBeenCalled();
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

// Mirrors quick-chat-modal.tsx's onEscapeKeyDown, itself invoked from Radix's
// DismissableLayer document-capture listener: runs the guard predicate BEFORE
// any target/bubble-phase consumer sees the event, so armedEventRef.current
// really does equal the dispatched event by the time consumers run -- the
// arrangement the round-4-only defaultPrevented/marker test at line 217
// cannot produce (no provider there, so the predicate never runs and
// armedEventRef stays null for every real dispatch).
function attachCaptureGuard(getGuard: () => ClarificationEscapeGuardEntry) {
  const onCapture = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    if (getGuard()?.test(event)) event.preventDefault();
  };
  document.addEventListener("keydown", onCapture, true);
  return () => document.removeEventListener("keydown", onCapture, true);
}

describe("ClarificationInputOverlay — Quick Chat capture-phase ordering (F6 regression)", () => {
  it("does not collapse when a consumer claims Escape via stopPropagation, even though the capture-phase guard armed it first (Quick Chat ordering)", () => {
    // Stands in for one of the F3-fixed consumers (tiptap-entity-reference-suggestion,
    // token-usage-display, message-history-search): an in-scope bubble-phase
    // listener between the target and window that calls stopPropagation() once
    // it claims the key. Unlike the F3 test at line 217 (preventDefault only,
    // no provider), this one runs the real predicate first via document
    // capture, so armedEventRef.current === the dispatched event -- the exact
    // condition under which the defaultPrevented/marker bail-out in
    // CarouselKeyboardShortcuts.onKey is a no-op (armedEventRef.current !== e
    // is false). Only the consumer's own stopPropagation() can still protect
    // the panel here, which is what this test actually proves.
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, scopeRef, onDismiss, getGuard } = renderOverlayWithGuard(messages);
    const detachGuard = attachCaptureGuard(getGuard);
    scopeRef.current!.addEventListener("keydown", (e) => {
      e.preventDefault();
      e.stopPropagation();
    });

    fireEvent.keyDown(composerRef.current!, { key: "Escape" });
    detachGuard();

    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("does collapse when the capture-phase guard arms the event and nothing downstream claims it (control case)", () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    const { composerRef, onDismiss, getGuard } = renderOverlayWithGuard(messages);
    const detachGuard = attachCaptureGuard(getGuard);

    fireEvent.keyDown(composerRef.current!, { key: "Escape" });
    detachGuard();

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});

describe("ClarificationInputOverlay — labelled Skip button", () => {
  it("still POSTs a rejection when the Skip button is clicked", async () => {
    const messages = [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })];
    renderOverlay(messages);

    fireEvent.click(screen.getByTestId(TESTID_SKIP));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      rejected: true,
      reject_reason: "User skipped",
    });
  });
});

// eslint-disable-next-line max-lines-per-function -- submit failure cases share one overlay harness.
describe("ClarificationInputOverlay — submit failure feedback", () => {
  it("shows a retry banner on a failed submit, preserves the answer, and never resolves", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const { onResolved } = renderOverlay([
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => screen.getByTestId(TESTID_SUBMIT_ERROR));
    expect(onResolved).not.toHaveBeenCalled();
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true");

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true }), { status: 200 }),
    );
    fireEvent.click(screen.getByTestId(TESTID_RETRY));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const [retryUrl, retryInit] = fetchMock.mock.calls[1];
    expect(String(retryUrl)).toBe("https://api.test/api/v1/clarification/p1/respond");
    expect(JSON.parse(String(retryInit.body))).toEqual({
      answers: [{ question_id: "q1", selected_options: ["o1"] }],
      rejected: false,
    });
    await vi.waitFor(() => expect(onResolved).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId(TESTID_SUBMIT_ERROR)).toBeNull();
  });

  it("restores Retry, Skip, and local Escape dismissal after a retryable 503", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: "clarification response is temporarily unavailable",
          code: "temporarily_unavailable",
        }),
        { status: 503, headers: { "Content-Type": "application/json" } },
      ),
    );
    const { onDismiss, scopeRef } = renderOverlay([
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();
    await vi.waitFor(() => expect(screen.getByTestId(TESTID_SUBMIT_ERROR)).toBeTruthy());

    expect(screen.getByTestId(TESTID_RETRY)).toBeTruthy();
    expect((screen.getByTestId(TESTID_SKIP) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("uses response-neutral copy when a Skip request fails", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })]);

    fireEvent.click(screen.getByTestId(TESTID_SKIP));

    await vi.waitFor(() => expect(screen.queryByTestId(TESTID_SUBMIT_ERROR)).not.toBeNull());
    expect(screen.getByTestId(TESTID_SUBMIT_ERROR).textContent).toContain(
      "Could not send your response.",
    );
  });

  it("retires an inactive bundle without leaving response controls mounted", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: "not_active", error: "clarification request is no longer active" }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );
    const { onResolved } = renderOverlay([
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => screen.getByTestId("clarification-expired"));
    expect(onResolved).not.toHaveBeenCalled();
    expect(screen.queryByTestId(TESTID_RETRY)).toBeNull();
    expect(screen.queryByTestId(TESTID_SKIP)).toBeNull();
    expect(screen.queryByTestId(TESTID_OPTION)).toBeNull();
    expect(screen.queryByTestId("clarification-submit")).toBeNull();
  });

  it("preserves affirmative answers and admits them as a new message after inactive", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ code: "not_active" }), { status: 409 }),
    );
    const onLateAnswer = vi.fn().mockResolvedValue("sent" as const);
    const { onResolved } = renderOverlay(
      [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })],
      { onLateAnswer },
    );

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(1));
    expect(onLateAnswer.mock.calls[0][0].answers).toEqual([
      { question_id: "q1", selected_options: ["o1"] },
    ]);
    expect(onResolved).not.toHaveBeenCalled();
    expect(screen.getByTestId("clarification-late-success").textContent).toContain(
      "Answer sent as a new message.",
    );
  });

  it("restores a retained late answer when the inline form is reopened", () => {
    const message = clarMessage({ id: "m-reopen", questionId: "q-reopen", index: 0, total: 1 });
    renderOverlay([message], {
      mode: "late",
      initialAnswers: [{ question_id: "q-reopen", selected_options: ["o1"] }],
      onLateAnswer: vi.fn().mockResolvedValue("sent" as const),
    });

    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true");
    expect((screen.getByTestId("clarification-late-submit") as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("keeps the active form and draft when late admission fails, then retries", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ code: "not_active" }), { status: 409 }),
    );
    const onLateAnswer = vi
      .fn()
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockResolvedValueOnce("queued" as const);
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })], {
      onLateAnswer,
    });

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();
    await vi.waitFor(() => expect(screen.getByTestId(TESTID_SUBMIT_ERROR)).toBeTruthy());
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true");

    fireEvent.click(screen.getByTestId(TESTID_RETRY));
    await vi.waitFor(() => expect(onLateAnswer).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => {
      expect(screen.getByTestId("clarification-late-success").textContent).toContain(
        "Answer queued as a new message.",
      );
    });
  });

  // Regression: a new bundle (different pending_id) arriving after a failed
  // submit must not render the previous bundle's stale banner over the live
  // question, and must not let Retry POST the old bundle's answers to the new
  // bundle's pendingId.
  it("clears the stale error banner when a new bundle (different pending_id) replaces a failed one", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const { scopeRef, onResolved, onDismiss, rerender } = renderOverlay([
      clarMessage({ id: "m-a", questionId: "qA", index: 0, total: 1, pendingId: "pA" }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();
    await vi.waitFor(() => screen.getByTestId(TESTID_SUBMIT_ERROR));

    rerender(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={[
            // Reusing a question ID must still reset the old bundle's local
            // retry state instead of treating it as the same answer.
            clarMessage({ id: "m-b", questionId: "qA", index: 0, total: 1, pendingId: "pB" }),
          ]}
          onResolved={onResolved}
          onDismiss={onDismiss}
          shortcutScopeRef={scopeRef}
        />
      </div>,
    );

    expect(screen.queryByTestId(TESTID_SUBMIT_ERROR)).toBeNull();
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).not.toBe("true");

    fetchMock.mockClear();
    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("https://api.test/api/v1/clarification/pB/respond");
  });
});

describe("ClarificationInputOverlay — outcome lifetime", () => {
  it("delivers a delayed losing outcome after the overlay unmounts", async () => {
    let resolveResponse: ((response: Response) => void) | undefined;
    fetchMock.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          resolveResponse = resolve;
        }),
    );
    const onOutcome = vi.fn();
    const { unmount } = renderOverlay(
      [clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })],
      { onOutcome },
    );

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    unmount();

    resolveResponse?.(
      new Response(
        JSON.stringify({
          success: true,
          claimed: false,
          status: "answered",
          response: { answers: [{ question_id: "q1", selected_options: ["o1"] }] },
        }),
        { status: 200 },
      ),
    );

    await vi.waitFor(() =>
      expect(onOutcome).toHaveBeenCalledWith({
        kind: "resolved",
        claimedByThisCaller: false,
        status: "answered",
      }),
    );
  });
});

describe("ClarificationInputOverlay — bundle-local state", () => {
  it("resets custom drafts and the active question when the bundle changes", async () => {
    const bundleA = [
      clarMessage({ id: "m-a1", questionId: "q1", index: 0, total: 2, pendingId: "pA" }),
      clarMessage({ id: "m-a2", questionId: "q2", index: 1, total: 2, pendingId: "pA" }),
    ];
    const bundleB = [
      clarMessage({ id: "m-b1", questionId: "q1", index: 0, total: 2, pendingId: "pB" }),
      clarMessage({ id: "m-b2", questionId: "q3", index: 1, total: 2, pendingId: "pB" }),
    ];
    const { rerender, scopeRef, onResolved, onDismiss } = renderOverlay(bundleA);

    fireEvent.change(screen.getByTestId(TESTID_INPUT), {
      target: { value: "stale draft" },
    });
    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[1]);
    expect(screen.getAllByTestId(TESTID_STEP)[1].getAttribute("data-active")).toBe("true");

    rerender(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={bundleB}
          onResolved={onResolved}
          onDismiss={onDismiss}
          shortcutScopeRef={scopeRef}
        />
      </div>,
    );

    await vi.waitFor(() =>
      expect(screen.getAllByTestId(TESTID_STEP)[0].getAttribute("data-active")).toBe("true"),
    );
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe("");
  });
});

describe("ClarificationInputOverlay — shared size control", () => {
  const messages = [
    clarMessage({ id: "m-size-1", questionId: "q1", index: 0, total: 2 }),
    clarMessage({ id: "m-size-2", questionId: "q2", index: 1, total: 2 }),
  ];

  it("renders one action-specific size toggle and forwards its action", () => {
    const onToggleSize = vi.fn();
    renderOverlay(messages, { onToggleSize, sizeExpanded: false, canExpandPanel: true });

    const toggle = screen.getByTestId(TESTID_SIZE_TOGGLE);
    expect(screen.getAllByTestId(TESTID_SIZE_TOGGLE)).toHaveLength(1);
    expect(toggle.getAttribute("aria-label")).toBe("Expand");
    expect(toggle.getAttribute("aria-disabled")).toBeNull();

    fireEvent.click(toggle);
    expect(onToggleSize).toHaveBeenCalledTimes(1);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows Restore as the next action while expanded and keeps it enabled without headroom", () => {
    const onToggleSize = vi.fn();
    renderOverlay(messages, { onToggleSize, sizeExpanded: true, canExpandPanel: false });

    const toggle = screen.getByTestId(TESTID_SIZE_TOGGLE) as HTMLButtonElement;
    expect(toggle.getAttribute("aria-label")).toBe("Restore");
    expect(toggle.getAttribute("aria-disabled")).toBeNull();

    fireEvent.click(toggle);
    expect(onToggleSize).toHaveBeenCalledTimes(1);
  });

  it("keeps Expand visible but disabled when the host has no usable headroom", () => {
    const onToggleSize = vi.fn();
    renderOverlay(messages, { onToggleSize, sizeExpanded: false, canExpandPanel: false });

    const toggle = screen.getByTestId(TESTID_SIZE_TOGGLE) as HTMLButtonElement;
    expect(toggle.getAttribute("aria-label")).toBe("Expand");
    expect(toggle.getAttribute("aria-disabled")).toBe("true");
    expect(toggle.disabled).toBe(false);

    fireEvent.click(toggle);
    expect(onToggleSize).not.toHaveBeenCalled();
  });
});

describe("ClarificationInputOverlay — lightweight Markdown", () => {
  it("renders question fields without changing the selected option payload", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const message = clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 });
    const metadata = message.metadata as ClarificationRequestMetadata;
    metadata.context =
      "Keep **context** readable.\n\n| Signal | Value |\n| --- | --- |\n| Ready | yes |\n\n[Context guide](https://example.com/context)";
    metadata.question = {
      id: "q1",
      title: "Use `fast` mode",
      prompt:
        "Choose **one** from the [storage guide](https://example.com/storage):\n\n1. First\n2. Second",
      options: [
        {
          option_id: "fast-mode",
          label: "Run in `fast` mode",
          description: "Best for **small** changes",
        },
      ],
    };
    renderOverlay([message]);

    const card = screen.getByTestId("clarification-question-card");
    const title = Array.from(card.querySelectorAll("span")).find(
      (element) => element.textContent === "Use fast mode" && element.querySelector("code"),
    );
    expect(title).toBeDefined();
    expect(card.querySelector("ol")?.textContent).toContain("First");
    const guidance = card.querySelector<HTMLAnchorElement>('a[href="https://example.com/storage"]');
    expect(guidance?.textContent).toBe("storage guide");
    expect(guidance?.target).toBe("_blank");

    const optionLabel = screen.getByTestId("clarification-option-label");
    const optionDescription = screen.getByTestId("clarification-option-description");
    expect(optionLabel.querySelector("code")?.textContent).toBe("fast");
    expect(optionDescription.querySelector("strong")?.textContent).toBe("small");

    const context = screen.getByTestId("clarification-context");
    expect(context.querySelector("strong")?.textContent).toBe("context");
    expect(context.querySelector("table")?.textContent).toContain("Ready");
    const contextGuide = context.querySelector<HTMLAnchorElement>(
      'a[href="https://example.com/context"]',
    );
    expect(contextGuide?.target).toBe("_blank");

    fireEvent.click(contextGuide!);
    expect(fetchMock).not.toHaveBeenCalled();

    fireEvent.click(optionLabel.querySelector("code")!);
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      answers: [{ question_id: "q1", selected_options: ["fast-mode"] }],
      rejected: false,
    });
  });
});

// AC .39: onOutcome is an additive, optional prop. Omitting it (as the task
// session and Quick Chat hosts both do) must leave every existing behavior
// -- including onResolved -- byte-identical to before this prop existed.
async function submitSingleOverlayAnswer() {
  const button = await screen.findByTestId("clarification-custom-submit");
  await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(button);
}

describe("ClarificationInputOverlay — onOutcome (AC .39)", () => {
  it("does not change onResolved or submit behavior when onOutcome is omitted", async () => {
    const { onResolved } = renderOverlay([
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(onResolved).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reports kind: resolved, claimedByThisCaller: true when this caller's own submit wins", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, claimed: true, status: "answered" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const onOutcome = vi.fn();
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })], {
      onOutcome,
    });

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() =>
      expect(onOutcome).toHaveBeenCalledWith({
        kind: "resolved",
        claimedByThisCaller: true,
        status: "answered",
      }),
    );
  });

  it("reports kind: resolved, claimedByThisCaller: false when another caller already answered", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          claimed: false,
          status: "answered",
          response: {
            answers: [{ question_id: "q1", selected_options: ["other-caller-option"] }],
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    const onOutcome = vi.fn();
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })], {
      onOutcome,
    });

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() =>
      expect(onOutcome).toHaveBeenCalledWith({
        kind: "resolved",
        claimedByThisCaller: false,
        status: "answered",
      }),
    );
  });

  it("reports kind: no_longer_active on a 409 not_active conflict", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ code: "not_active", error: "clarification request is no longer active" }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );
    const onOutcome = vi.fn();
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })], {
      onOutcome,
    });

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(onOutcome).toHaveBeenCalledWith({ kind: "no_longer_active" }));
  });

  it("reports kind: submission_failed on a network/server failure", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const onOutcome = vi.fn();
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })], {
      onOutcome,
    });

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await submitSingleOverlayAnswer();

    await vi.waitFor(() => expect(onOutcome).toHaveBeenCalledWith({ kind: "submission_failed" }));
  });
});

describe("ClarificationInputOverlay — explicit single-question submission", () => {
  it("keeps a choice pending until Send and sends its restored files with the answer", async () => {
    const attachment = {
      attachment_id: "att-restored",
      type: "image",
      name: "evidence.png",
      mime_type: "image/png",
      size_bytes: 3,
      delivery_mode: "path",
    };
    const messages = [
      clarMessage({
        id: "m1",
        questionId: "q1",
        index: 0,
        total: 1,
        attachments: [attachment],
      }),
    ];
    renderOverlay(messages);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, claimed: true, status: "answered" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true");

    await submitSingleOverlayAnswer();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      answers: [
        {
          question_id: "q1",
          selected_options: ["o1"],
          attachments: [attachment],
        },
      ],
      rejected: false,
    });
  });
});

const DRAFT_DATA_ACTIVE = "data-active";
type RetainedViewOptions = {
  taskId?: string;
  onResolved?: () => void;
  mode?: "active" | "late";
};

function retainedBundle(pendingId = "p1") {
  return [
    clarMessage({ id: "m-q1", questionId: "q1", index: 0, total: 2, pendingId }),
    clarMessage({ id: "m-q2", questionId: "q2", index: 1, total: 2, pendingId }),
  ];
}

function retainedView() {
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = vi.fn();
  const onDismiss = vi.fn();
  const view = (messages: Message[], options: RetainedViewOptions = {}) => (
    <ClarificationDraftProvider taskId={options.taskId ?? "task-a"}>
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={messages}
          onResolved={options.onResolved ?? onResolved}
          onDismiss={onDismiss}
          mode={options.mode}
          shortcutScopeRef={scopeRef}
        />
      </div>
    </ClarificationDraftProvider>
  );
  return { view, scopeRef, onResolved, onDismiss };
}

function enterRetainedAnswers() {
  fireEvent.click(screen.getByTestId(TESTID_OPTION));
  fireEvent.change(screen.getByTestId(TESTID_INPUT), {
    target: { value: "Use Go for the service" },
  });
}

describe("ClarificationInputOverlay — retained answer drafts", () => {
  beforeEach(() => {
    clearAnswerDrafts(mockStore);
  });

  it("restores keyed selections, unfinished custom text, and the question after unmount", async () => {
    const { view } = retainedView();
    const bundle = retainedBundle();
    const { rerender } = render(view(bundle));
    enterRetainedAnswers();

    rerender(<div />);
    rerender(view(bundle));

    await vi.waitFor(() =>
      expect(screen.getAllByTestId(TESTID_STEP)[1].getAttribute(DRAFT_DATA_ACTIVE)).toBe("true"),
    );
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe(
      "Use Go for the service",
    );
    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[0]);
    await vi.waitFor(() =>
      expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true"),
    );
  });

  it("rejects question and option identities the current bundle no longer contains", async () => {
    const { view } = retainedView();
    const { rerender } = render(view(retainedBundle()));
    enterRetainedAnswers();
    rerender(<div />);

    const changed = [
      clarMessage({ id: "m-q1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ id: "m-q3", questionId: "q3", index: 1, total: 2 }),
    ];
    const question = (changed[0].metadata as ClarificationRequestMetadata).question;
    question.options = [{ option_id: "o2", label: "Option 2", description: "" }];
    rerender(view(changed));

    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[0]);
    await vi.waitFor(() =>
      expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).not.toBe("true"),
    );
    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[1]);
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe("");
  });

  it("does not leak the previous bundle's answers into a replacement bundle", async () => {
    const { view } = retainedView();
    const { rerender } = render(view(retainedBundle("pA")));
    enterRetainedAnswers();

    const replacement = retainedBundle("pB");
    rerender(view(replacement));
    rerender(<div />);
    rerender(view(replacement));

    expect(screen.getAllByTestId(TESTID_STEP)[0].getAttribute(DRAFT_DATA_ACTIVE)).toBe("true");
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe("");
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).not.toBe("true");
  });

  it("preserves the exact raw custom text across restoration and submission", async () => {
    const { view } = retainedView();
    const bundle = retainedBundle("p-raw");
    const { rerender } = render(view(bundle, { taskId: "task-raw" }));
    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    const rawDraft = "  Use Go\n";
    fireEvent.change(screen.getByTestId(TESTID_INPUT), { target: { value: rawDraft } });
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe(rawDraft);

    rerender(<div />);
    rerender(view(bundle, { taskId: "task-raw" }));

    await vi.waitFor(() =>
      expect(screen.getAllByTestId(TESTID_STEP)[1].getAttribute(DRAFT_DATA_ACTIVE)).toBe("true"),
    );
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe(rawDraft);

    const submit = screen.getByTestId("clarification-submit") as HTMLButtonElement;
    await vi.waitFor(() => expect(submit.disabled).toBe(false));
    fireEvent.click(submit);
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const body = JSON.parse(String(fetchMock.mock.calls[0][1]?.body)) as {
      answers: Array<{ question_id: string; custom_text?: string }>;
    };
    expect(body.answers.find((answer) => answer.question_id === "q2")?.custom_text).toBe("Use Go");
  });

  it("retains answers for questions that have not arrived yet", async () => {
    const { view } = retainedView();
    const bundle = retainedBundle("p-partial");
    const { rerender } = render(view(bundle));
    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    fireEvent.change(screen.getByTestId(TESTID_INPUT), {
      target: { value: "  Use Go\n" },
    });

    // A partial bundle must not filter the not-yet-arrived answer out of the
    // cache before the remaining question renders.
    rerender(view([bundle[0]]));
    await vi.waitFor(() =>
      expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true"),
    );
    rerender(<div />);
    rerender(view(bundle));

    await vi.waitFor(() =>
      expect(screen.getAllByTestId(TESTID_STEP)[1].getAttribute(DRAFT_DATA_ACTIVE)).toBe("true"),
    );
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe("  Use Go\n");
    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[0]);
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).toBe("true");
  });
});

describe("ClarificationInputOverlay — retained answer draft isolation", () => {
  beforeEach(() => {
    clearAnswerDrafts(mockStore);
  });

  it("keeps late-answer forms out of the shared navigation cache", () => {
    const { view } = retainedView();
    const bundle = retainedBundle("p-late");
    const { rerender } = render(view(bundle, { taskId: "task-late", mode: "late" }));
    fireEvent.click(screen.getByTestId(TESTID_OPTION));
    fireEvent.change(screen.getByTestId(TESTID_INPUT), {
      target: { value: "late draft" },
    });

    rerender(<div />);
    rerender(view(bundle, { taskId: "task-late", mode: "late" }));

    expect(screen.getAllByTestId(TESTID_STEP)[0].getAttribute(DRAFT_DATA_ACTIVE)).toBe("true");
    expect((screen.getByTestId(TESTID_INPUT) as HTMLTextAreaElement).value).toBe("");
    expect(screen.getByTestId(TESTID_OPTION).getAttribute(DATA_SELECTED)).not.toBe("true");
  });

  it("does not retain an empty form mount", () => {
    const { view } = retainedView();
    render(view(retainedBundle("p-empty"), { taskId: "task-empty" }));

    const identity = answerDraftIdentity("task-empty", JSON.stringify(["s1", "p-empty", "active"]));
    expect(readAnswerDraft(mockStore, identity)).toBeUndefined();
  });
});
