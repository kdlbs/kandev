import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { i18n } from "@/lib/i18n";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationRequestMetadata,
  type Message,
} from "@/lib/types/http";
import { ClarificationPanelSection } from "./clarification-panel-section";
import { ClarificationHeaderActions } from "./clarification-overlay-header";
import {
  ComposerDisclosureContext,
  type ComposerDisclosureContextValue,
} from "./composer-disclosure";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

const mockUpdateMessage = vi.fn();
const imageMocks = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("@/lib/api/domains/question-image-api", () => ({
  fetchQuestionImage: imageMocks.fetch,
  questionImageErrorCode: () => "unavailable",
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { mode: "none", authenticated: true, user: null } }),
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
  pendingId: string;
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
      pending_id: opts.pendingId,
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

function setTestRect(element: HTMLElement, top: number, height: number) {
  element.getBoundingClientRect = () =>
    ({ top, bottom: top + height, left: 0, right: 800, width: 800, height }) as DOMRect;
}

function renderSection(
  pending: boolean,
  messages: Message[],
  initialMaxHeightVh = 50,
  host: { allocationRef?: React.RefObject<HTMLElement | null>; reservedHeightPx?: number } = {},
) {
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = vi.fn();
  const utils = render(
    <div ref={scopeRef} tabIndex={-1}>
      <ClarificationPanelSection
        pending={pending}
        messages={messages}
        onResolved={onResolved}
        shortcutScopeRef={scopeRef}
        initialMaxHeightVh={initialMaxHeightVh}
        allocationRef={host.allocationRef}
        reservedHeightPx={host.reservedHeightPx}
      />
    </div>,
  );
  return { ...utils, scopeRef, onResolved };
}

function renderThreadSection(messages: Message[], allocation: HTMLElement) {
  const scopeRef = createRef<HTMLDivElement>();
  const allocationRef = { current: allocation } as React.RefObject<HTMLElement | null>;
  const disclosure = {
    clarificationAllocationRef: allocationRef,
    clarificationReservedHeightPx: 0,
  } as ComposerDisclosureContextValue;
  const utils = render(
    <ComposerDisclosureContext.Provider value={disclosure}>
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationPanelSection
          pending
          messages={messages}
          onResolved={vi.fn()}
          shortcutScopeRef={scopeRef}
          initialMaxHeightVh={50}
        />
      </div>
    </ComposerDisclosureContext.Provider>,
  );
  return { ...utils, scopeRef };
}

beforeEach(() => {
  Object.defineProperty(window, "innerHeight", { configurable: true, writable: true, value: 1000 });
  fetchMock.mockReset();
  imageMocks.fetch.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
});

afterEach(async () => {
  cleanup();
  vi.unstubAllGlobals();
  // Restoring the locale is load-bearing: changeLanguage mutates the shared
  // instance vitest.setup.ts initializes, so leaving it on pseudo would leak
  // into every test that runs after this file.
  await i18n.changeLanguage("en");
});

const SCROLL_REGION_TESTID = "clarification-scroll-region";
const OPTION_TESTID = "clarification-option";
const CONTAINER_TESTID = "clarification-overlay-container";
const QUESTION_COUNT_TESTID = "clarification-question-count";
const SUBMITTING_STATUS_TESTID = "clarification-submitting-status";

describe("ClarificationPanelSection — image lifecycle", () => {
  it("cancels hidden question and choice reads while preserving the answer draft", async () => {
    const images = [1, 2, 3, 4].map((id) => ({
      id: `image${id}`,
      path: `${id}.png`,
      alt: `Preview ${id}`,
    }));
    const message = clarMessage({
      pendingId: "images",
      id: "m1",
      questionId: "q1",
      index: 0,
      total: 1,
    });
    const metadata = message.metadata as ClarificationRequestMetadata;
    metadata.question.images = images.slice(0, 2);
    metadata.question.options = images.slice(2).map((image) => ({
      option_id: image.id,
      label: image.alt,
      description: "",
      images: [image],
    }));
    const pending: { signal: AbortSignal; resolve: (blob: Blob) => void }[] = [];
    imageMocks.fetch.mockImplementation(
      (_pending, _question, _image, signal: AbortSignal) =>
        new Promise<Blob>((resolve) => pending.push({ signal, resolve })),
    );
    vi.stubGlobal("URL", {
      createObjectURL: vi.fn(() => "blob:preview"),
      revokeObjectURL: vi.fn(),
    });
    const { unmount, onResolved } = renderSection(true, [message]);
    try {
      await waitFor(() => expect(imageMocks.fetch).toHaveBeenCalledTimes(2));
      fireEvent.change(screen.getByTestId("clarification-input"), {
        target: { value: "Keep this answer draft." },
      });
      fireEvent.click(screen.getByRole("button", { name: "Collapse clarification" }));
      expect(pending.every(({ signal }) => signal.aborted)).toBe(true);
      await act(async () => pending.forEach(({ resolve }) => resolve(new Blob(["late"]))));
      expect(imageMocks.fetch).toHaveBeenCalledTimes(2);
      expect(URL.createObjectURL).not.toHaveBeenCalled();
      expect(fetchMock).not.toHaveBeenCalled();
      expect(onResolved).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole("button", { name: "Expand clarification" }));
      expect((screen.getByTestId("clarification-input") as HTMLTextAreaElement).value).toBe(
        "Keep this answer draft.",
      );
      await waitFor(() => expect(imageMocks.fetch).toHaveBeenCalledTimes(4));
      await act(async () =>
        pending.slice(2).forEach(({ resolve }) => resolve(new Blob(["fresh"]))),
      );
      await waitFor(() => expect(imageMocks.fetch).toHaveBeenCalledTimes(6));
      await act(async () =>
        pending.slice(4).forEach(({ resolve }) => resolve(new Blob(["choice"]))),
      );
      expect(screen.getAllByRole("img")).toHaveLength(4);
      fireEvent.click(screen.getByRole("button", { name: "Collapse clarification" }));
      expect(URL.revokeObjectURL).toHaveBeenCalledTimes(4);
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      unmount();
      await act(async () => pending.forEach(({ resolve }) => resolve(new Blob(["cleanup"]))));
    }
  });
});

describe("ClarificationPanelSection — collapse affordance", () => {
  it("keeps the expanded question controls in one header row", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 3 }),
      clarMessage({ pendingId: "p1", id: "m2", questionId: "q2", index: 1, total: 3 }),
      clarMessage({ pendingId: "p1", id: "m3", questionId: "q3", index: 2, total: 3 }),
    ];
    renderSection(true, messages);

    const header = screen.getByTestId("clarification-overlay-header");
    expect(header.querySelector('[data-testid="clarification-stepper"]')).not.toBeNull();
    expect(header.querySelector('[data-testid="clarification-submit"]')).not.toBeNull();
    expect(header.querySelector('[data-testid="clarification-skip"]')).not.toBeNull();
    expect(header.querySelector('[data-testid="clarification-collapse-toggle"]')).not.toBeNull();
    expect(screen.queryByText("Clarification needed")).toBeNull();
  });

  it("collapses on Escape without posting a rejection, and stays reachable to expand again", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).not.toContain("hidden");

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId(CONTAINER_TESTID)).not.toBeNull();
    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).toContain("hidden");

    // Restore: clicking the toggle re-expands the same still-pending bundle.
    fireEvent.click(screen.getByRole("button", { name: "Expand clarification" }));
    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).not.toContain("hidden");
  });

  it("the labelled Skip button inside an expanded section still posts a rejection", async () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages);

    fireEvent.click(screen.getByTestId("clarification-skip"));

    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });

  it("resets to expanded when a new bundle (different pending_id) arrives while collapsed", () => {
    const bundleA = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    const bundleB = [
      clarMessage({ pendingId: "p2", id: "m2", questionId: "q2", index: 0, total: 1 }),
    ];
    const { rerender, scopeRef } = renderSection(true, bundleA);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });
    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).toContain("hidden");

    rerender(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationPanelSection
          pending={true}
          messages={bundleB}
          onResolved={vi.fn()}
          shortcutScopeRef={scopeRef}
          initialMaxHeightVh={50}
        />
      </div>,
    );

    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).not.toContain("hidden");
  });
});

describe("ClarificationPanelSection — submitting feedback", () => {
  it("keeps the multi-question header status announced with a stable Submit name", () => {
    render(
      <ClarificationHeaderActions
        total={3}
        allAnswered={true}
        isSubmitting={true}
        onSubmit={vi.fn()}
        onSkip={vi.fn()}
      />,
    );

    const status = screen.getByTestId(SUBMITTING_STATUS_TESTID);
    const submit = screen.getByTestId("clarification-submit");
    expect(status.getAttribute("aria-label")).toBe(i18n.t("task:submitting"));
    expect(status.getAttribute("aria-hidden")).toBeNull();
    expect(submit.getAttribute("aria-label")).toBe(i18n.t("task:submit"));
  });

  it("shows the translated submitting status before Skip while a single answer is in flight", async () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    let releaseResponse!: (response: Response) => void;
    const heldResponse = new Promise<Response>((resolve) => {
      releaseResponse = resolve;
    });
    fetchMock.mockReturnValueOnce(heldResponse);
    renderSection(true, messages);

    expect(screen.queryByTestId(SUBMITTING_STATUS_TESTID)).toBeNull();

    try {
      fireEvent.click(screen.getByTestId(OPTION_TESTID));

      const status = await screen.findByTestId(SUBMITTING_STATUS_TESTID);
      const skip = screen.getByTestId("clarification-skip");
      expect(status.getAttribute("aria-label")).toBe(i18n.t("task:submitting"));
      expect(status.getAttribute("aria-hidden")).toBeNull();
      expect(status.compareDocumentPosition(skip) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect((skip as HTMLButtonElement).disabled).toBe(true);

      releaseResponse(
        new Response(JSON.stringify({ success: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      await vi.waitFor(() => expect(screen.queryByTestId(SUBMITTING_STATUS_TESTID)).toBeNull());
    } finally {
      releaseResponse(
        new Response(JSON.stringify({ success: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
  });

  it("removes the submitting status and re-enables Skip when the answer request fails", async () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    let rejectResponse!: (error: Error) => void;
    const heldResponse = new Promise<Response>((_, reject) => {
      rejectResponse = reject;
    });
    fetchMock.mockReturnValueOnce(heldResponse);
    renderSection(true, messages);

    try {
      fireEvent.click(screen.getByTestId(OPTION_TESTID));
      await screen.findByTestId(SUBMITTING_STATUS_TESTID);

      rejectResponse(new Error("network"));
      await vi.waitFor(() => {
        expect(screen.queryByTestId(SUBMITTING_STATUS_TESTID)).toBeNull();
        expect((screen.getByTestId("clarification-skip") as HTMLButtonElement).disabled).toBe(
          false,
        );
      });
    } finally {
      rejectResponse(new Error("network"));
    }
  });
});

describe("ClarificationPanelSection — per-surface initial height cap", () => {
  it("renders the initial cap as pixels before the host budget is expanded", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 50);

    const container = screen.getByTestId(CONTAINER_TESTID);
    expect(container.style.maxHeight).toBe("500px");
  });

  it("keeps a different surface's initial ratio (e.g. Quick Chat's 35vh)", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 35);

    const container = screen.getByTestId(CONTAINER_TESTID);
    expect(container.style.maxHeight).toBe("350px");
  });

  it("expands to measured host space and resets to content sizing without losing an answer draft", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages);
    const container = screen.getByTestId(CONTAINER_TESTID);
    const input = screen.getByTestId("clarification-input") as HTMLTextAreaElement;
    fireEvent.change(input, { target: { value: "Keep this answer." } });

    const expand = screen.getByTestId("clarification-panel-expand") as HTMLButtonElement;
    expect(expand.disabled).toBe(false);
    fireEvent.click(expand);

    expect(container.style.height).toBe("1000px");
    expect(container.style.maxHeight).toBe("1000px");
    expect(input.value).toBe("Keep this answer.");
    expect(fetchMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId("clarification-panel-reset"));

    expect(container.style.height).toBe("");
    expect(container.style.maxHeight).toBe("500px");
    expect(input.value).toBe("Keep this answer.");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("disables Expand when the host has no unreserved space", () => {
    const allocation = document.createElement("div");
    document.body.append(allocation);
    setTestRect(allocation, 0, 100);
    const allocationRef = { current: allocation } as React.RefObject<HTMLElement | null>;
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 50, { allocationRef, reservedHeightPx: 100 });

    expect((screen.getByTestId("clarification-panel-expand") as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getByTestId(CONTAINER_TESTID).getAttribute("data-host-max-height")).toBe("0");
    expect((screen.getByTestId(CONTAINER_TESTID) as HTMLDivElement).style.minHeight).toBe("0px");
  });

  it("keeps the viewport fallback minimum while its host allocation is unavailable", () => {
    const allocation = document.createElement("div");
    const allocationRef = { current: allocation } as React.RefObject<HTMLElement | null>;
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 50, { allocationRef });

    const container = screen.getByTestId(CONTAINER_TESTID) as HTMLDivElement;
    expect(container.dataset.hostMaxHeight).toBe("");
    expect(container.style.minHeight).toBe("120px");
  });

  it("uses a non-Threads host budget below the normal panel minimum", () => {
    const allocation = document.createElement("div");
    document.body.append(allocation);
    setTestRect(allocation, 0, 150);
    const allocationRef = { current: allocation } as React.RefObject<HTMLElement | null>;
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 50, { allocationRef, reservedHeightPx: 100 });

    const container = screen.getByTestId(CONTAINER_TESTID) as HTMLDivElement;
    expect(container.dataset.hostMaxHeight).toBe("50");
    expect(container.style.maxHeight).toBe("50px");
    expect(container.style.minHeight).toBe("50px");
  });

  it("omits manual height while Threads uses the footer scroll fallback", async () => {
    const allocation = document.createElement("div");
    document.body.append(allocation);
    setTestRect(allocation, 0, 800);
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderThreadSection(messages, allocation);

    const container = screen.getByTestId(CONTAINER_TESTID) as HTMLDivElement;
    fireEvent.click(screen.getByTestId("clarification-panel-expand"));
    expect(container.style.height).toBe("800px");

    setTestRect(allocation, 0, 50);
    act(() => window.dispatchEvent(new Event("resize")));
    await waitFor(() => expect(container.dataset.hostMaxHeight).toBe("50"));

    expect(container.dataset.scrollOwner).toBe("thread-footer");
    expect(container.style.height).toBe("");
  });
});

describe("ClarificationPanelSection — question-count localization", () => {
  it("uses the singular form for exactly one question in the bundle", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 2 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(screen.getByTestId(QUESTION_COUNT_TESTID).textContent).toBe("1 question");
  });

  it("uses the plural form for more than one question in the bundle", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ pendingId: "p1", id: "m2", questionId: "q2", index: 1, total: 2 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(screen.getByTestId(QUESTION_COUNT_TESTID).textContent).toBe("2 questions");
  });

  it("keeps the total count wording after a partial answer", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 2 }),
      clarMessage({ pendingId: "p1", id: "m2", questionId: "q2", index: 1, total: 2 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    fireEvent.click(screen.getAllByTestId(OPTION_TESTID)[0]);
    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(screen.getByTestId(QUESTION_COUNT_TESTID).textContent).toBe("2 questions");
  });

  it("resolves the question-count text through the catalog on a locale switch, not an English fallback", async () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    await i18n.changeLanguage("pseudo");

    expect(screen.getByTestId(QUESTION_COUNT_TESTID).textContent).toBe("1 qũēśţĩōń");
  });
});

describe("ClarificationPanelSection — dragged height does not survive a bundle handover", () => {
  it("resets the user-dragged height when a new bundle (different pending_id) replaces one still pending", () => {
    const bundleA = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    const bundleB = [
      clarMessage({ pendingId: "p2", id: "m2", questionId: "q2", index: 0, total: 1 }),
    ];
    const { rerender, scopeRef } = renderSection(true, bundleA);
    const container = screen.getByTestId(CONTAINER_TESTID);

    const handle = screen.getByRole("button", { name: /resize/i });
    fireEvent.mouseDown(handle, { clientY: 500 });
    fireEvent.mouseMove(document, { clientY: 300 }); // drag up 200px → explicit pixel height
    fireEvent.mouseUp(document);

    expect(container.style.height).toMatch(/^\d+(\.\d+)?px$/);

    rerender(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationPanelSection
          pending={true}
          messages={bundleB}
          onResolved={vi.fn()}
          shortcutScopeRef={scopeRef}
          initialMaxHeightVh={50}
        />
      </div>,
    );

    // Bundle B must not inherit bundle A's dragged pixel height.
    expect(container.style.height).toBe("");
  });
});

// AC .39: onOutcome is additive and forwarded straight through to
// ClarificationInputOverlay. Existing callers (task chat, Quick Chat) omit
// it and must see no behavior change; a new caller (the Needs-you Inbox)
// must receive every settled outcome.
describe("ClarificationPanelSection — onOutcome forwarding (AC .39)", () => {
  it("forwards a resolved outcome to onOutcome when provided", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ success: true, claimed: true, status: "answered" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    const onOutcome = vi.fn();
    const scopeRef = createRef<HTMLDivElement>();
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    render(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationPanelSection
          pending={true}
          messages={messages}
          onResolved={vi.fn()}
          onOutcome={onOutcome}
          shortcutScopeRef={scopeRef}
          initialMaxHeightVh={50}
        />
      </div>,
    );

    fireEvent.click(screen.getByTestId(OPTION_TESTID));

    await vi.waitFor(() =>
      expect(onOutcome).toHaveBeenCalledWith({
        kind: "resolved",
        claimedByThisCaller: true,
        status: "answered",
      }),
    );
  });

  it("behaves identically to before when onOutcome is omitted", async () => {
    const { onResolved } = renderSection(true, [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ]);

    fireEvent.click(screen.getByTestId(OPTION_TESTID));

    await vi.waitFor(() => expect(onResolved).toHaveBeenCalledTimes(1));
  });
});
