import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent, screen } from "@testing-library/react";
import { i18n } from "@/lib/i18n";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ClarificationPanelSection } from "./clarification-panel-section";
import { ClarificationHeaderActions } from "./clarification-overlay-header";

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

function renderSection(pending: boolean, messages: Message[], maxHeightVh = 50) {
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = vi.fn();
  const utils = render(
    <div ref={scopeRef} tabIndex={-1}>
      <ClarificationPanelSection
        pending={pending}
        messages={messages}
        onResolved={onResolved}
        shortcutScopeRef={scopeRef}
        maxHeightVh={maxHeightVh}
      />
    </div>,
  );
  return { ...utils, scopeRef, onResolved };
}

beforeEach(() => {
  fetchMock.mockReset();
  mockUpdateMessage.mockReset();
  fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
});

afterEach(async () => {
  cleanup();
  // Restoring the locale is load-bearing: changeLanguage mutates the shared
  // instance vitest.setup.ts initializes, so leaving it on pseudo would leak
  // into every test that runs after this file.
  await i18n.changeLanguage("en");
});

const SCROLL_REGION_TESTID = "clarification-scroll-region";
const CONTAINER_TESTID = "clarification-overlay-container";
const QUESTION_COUNT_TESTID = "clarification-question-count";
const SUBMITTING_STATUS_TESTID = "clarification-submitting-status";

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
          maxHeightVh={50}
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
      fireEvent.click(screen.getByTestId("clarification-option"));

      const status = await screen.findByTestId(SUBMITTING_STATUS_TESTID);
      const skip = screen.getByTestId("clarification-skip");
      expect(status.getAttribute("aria-label")).toBe(i18n.t("task:submitting"));
      expect(status.getAttribute("aria-hidden")).toBeNull();
      expect(status.compareDocumentPosition(skip) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect((skip as HTMLButtonElement).disabled).toBe(true);

      releaseResponse(
        new Response("{}", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      await vi.waitFor(() => expect(screen.queryByTestId(SUBMITTING_STATUS_TESTID)).toBeNull());
    } finally {
      releaseResponse(
        new Response("{}", {
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
      fireEvent.click(screen.getByTestId("clarification-option"));
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

describe("ClarificationPanelSection — per-surface max-height cap", () => {
  it("renders the CSS max-height from the maxHeightVh prop instead of a hardcoded value", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 50);

    const container = screen.getByTestId(CONTAINER_TESTID);
    expect(container.style.maxHeight).toBe("50vh");
  });

  it("a different surface's maxHeightVh renders a different cap (e.g. Quick Chat's 35vh)", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    renderSection(true, messages, 35);

    const container = screen.getByTestId(CONTAINER_TESTID);
    expect(container.style.maxHeight).toBe("35vh");
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

    fireEvent.click(screen.getAllByTestId("clarification-option")[0]);
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
          maxHeightVh={50}
        />
      </div>,
    );

    // Bundle B must not inherit bundle A's dragged pixel height.
    expect(container.style.height).toBe("");
  });
});
