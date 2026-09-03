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
const TESTID_OPTION = "clarification-option";
const TESTID_STEP = "clarification-step";
const TESTID_SUBMIT_ERROR = "clarification-submit-error";

function clarMessage(opts: {
  id: string;
  questionId: string;
  index: number;
  total: number;
  pendingId?: string;
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

    fireEvent.click(screen.getAllByTestId(TESTID_STEP)[1]);
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

    fireEvent.click(screen.getByTestId("clarification-skip"));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      rejected: true,
      reject_reason: "User skipped",
    });
  });
});

describe("ClarificationInputOverlay — submit failure feedback", () => {
  it("shows a retry banner on a failed submit, preserves the answer, and never resolves", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    const { onResolved } = renderOverlay([
      clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(TESTID_OPTION));

    await vi.waitFor(() => screen.getByTestId(TESTID_SUBMIT_ERROR));
    expect(onResolved).not.toHaveBeenCalled();
    expect(screen.getByTestId(TESTID_OPTION).getAttribute("data-selected")).toBe("true");

    fetchMock.mockResolvedValueOnce(new Response(null, { status: 200 }));
    fireEvent.click(screen.getByTestId("clarification-retry"));

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

  it("uses response-neutral copy when a Skip request fails", async () => {
    fetchMock.mockResolvedValueOnce(new Response("nope", { status: 500 }));
    renderOverlay([clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 })]);

    fireEvent.click(screen.getByTestId("clarification-skip"));

    await vi.waitFor(() => expect(screen.queryByTestId(TESTID_SUBMIT_ERROR)).not.toBeNull());
    expect(screen.getByTestId(TESTID_SUBMIT_ERROR).textContent).toContain(
      "Could not send your response.",
    );
  });

  it("shows a non-retryable expired banner when the bundle is no longer active", async () => {
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

    await vi.waitFor(() => screen.getByTestId("clarification-expired"));
    expect(onResolved).not.toHaveBeenCalled();
    expect(screen.queryByTestId("clarification-retry")).toBeNull();
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
    expect(screen.getByTestId(TESTID_OPTION).getAttribute("data-selected")).not.toBe("true");

    fetchMock.mockClear();
    fireEvent.click(screen.getByTestId(TESTID_OPTION));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("https://api.test/api/v1/clarification/pB/respond");
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

    fireEvent.change(screen.getByTestId("clarification-input"), {
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
    expect((screen.getByTestId("clarification-input") as HTMLTextAreaElement).value).toBe("");
  });
});

describe("ClarificationInputOverlay — lightweight Markdown", () => {
  it("renders question fields without changing the selected option payload", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
    );
    const message = clarMessage({ id: "m1", questionId: "q1", index: 0, total: 1 });
    const metadata = message.metadata as ClarificationRequestMetadata;
    metadata.context = "Keep `context` literal";
    metadata.question = {
      id: "q1",
      title: "Use `fast` mode",
      prompt: "Choose **one**:\n\n1. First\n2. Second",
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

    const optionLabel = screen.getByTestId("clarification-option-label");
    const optionDescription = screen.getByTestId("clarification-option-description");
    expect(optionLabel.querySelector("code")?.textContent).toBe("fast");
    expect(optionDescription.querySelector("strong")?.textContent).toBe("small");

    const context = screen.getByTestId("clarification-context");
    expect(context.textContent).toBe("Keep `context` literal");
    expect(context.querySelector("code")).toBeNull();

    fireEvent.click(optionLabel.querySelector("code")!);

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init.body))).toEqual({
      answers: [{ question_id: "q1", selected_options: ["fast-mode"] }],
      rejected: false,
    });
  });
});
