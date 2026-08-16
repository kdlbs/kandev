import { createRef } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, fireEvent, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ClarificationPanelSection } from "./clarification-panel-section";

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

function renderSection(pending: boolean, messages: Message[]) {
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = vi.fn();
  const utils = render(
    <div ref={scopeRef} tabIndex={-1}>
      <ClarificationPanelSection
        pending={pending}
        messages={messages}
        onResolved={onResolved}
        shortcutScopeRef={scopeRef}
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

afterEach(() => {
  cleanup();
});

const SCROLL_REGION_TESTID = "clarification-scroll-region";

describe("ClarificationPanelSection — collapse affordance", () => {
  it("collapses on Escape without posting a rejection, and stays reachable to expand again", () => {
    const messages = [
      clarMessage({ pendingId: "p1", id: "m1", questionId: "q1", index: 0, total: 1 }),
    ];
    const { scopeRef } = renderSection(true, messages);

    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).not.toContain("hidden");

    fireEvent.keyDown(scopeRef.current!, { key: "Escape" });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("clarification-overlay-container")).not.toBeNull();
    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).toContain("hidden");

    // Restore: clicking the toggle re-expands the same still-pending bundle.
    fireEvent.click(screen.getByTestId("clarification-collapse-toggle"));
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
        />
      </div>,
    );

    expect(screen.getByTestId(SCROLL_REGION_TESTID).className).not.toContain("hidden");
  });
});
