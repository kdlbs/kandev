import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SessionUsageTotals, UsageTurn } from "@/lib/types/conversation-usage";

const mocks = vi.hoisted(() => ({
  touch: true,
  loadTurnDetail: vi.fn(),
  usage: null as unknown,
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => mocks.touch }));
vi.mock("@/hooks/domains/session/use-conversation-usage", () => ({
  useConversationUsage: () =>
    mocks.usage ?? {
      totals: {
        scope: "session",
        scope_id: "session-1",
        tokens_total: 1200,
        cost_subcents_decimal: "150000",
        event_count: 1,
        unpriced_event_count: 0,
        estimated_event_count: 0,
      } as SessionUsageTotals,
      latestTurn: {
        turn_id: "turn-1",
        completeness: "exact",
        direct: {
          input_tokens: 400,
          cached_read_tokens: 600,
          cached_write_tokens: 0,
          output_tokens: 200,
          thought_tokens: 0,
          total_tokens: 1200,
          output_complete: true,
          event_count: 1,
          unpriced_count: 0,
          cost_subcents: "150000",
          overflow: false,
        },
        child: {
          input_tokens: 0,
          cached_read_tokens: 0,
          cached_write_tokens: 0,
          output_tokens: 0,
          thought_tokens: 0,
          total_tokens: 0,
          output_complete: true,
          event_count: 0,
          unpriced_count: 0,
          overflow: false,
        },
        total: {
          input_tokens: 400,
          cached_read_tokens: 600,
          cached_write_tokens: 0,
          output_tokens: 200,
          thought_tokens: 0,
          total_tokens: 1200,
          output_complete: true,
          event_count: 1,
          unpriced_count: 0,
          cost_subcents: "150000",
          overflow: false,
        },
        cost_sources: ["models_dev_list"],
        last_response: {
          usage_event_id: "usage-1",
          provider_response_id: "response-1",
          input_tokens: 1000,
          cached_read_tokens: 600,
          output_tokens: 200,
          reasoning_output_tokens: 80,
          total_tokens: 1200,
          cost_source: "models_dev_list",
          estimated: false,
          occurred_at: "2026-09-24T00:00:00Z",
        },
      } as UsageTurn,
      detail: null,
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    },
}));
vi.mock("@kandev/ui/drawer", () => ({
  DrawerClose: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    open,
    title,
    children,
    headerAction,
    contentTestId,
  }: {
    open: boolean;
    title: string;
    children: React.ReactNode;
    headerAction?: React.ReactNode;
    contentTestId?: string;
  }) =>
    open ? (
      <section role="dialog" aria-label={title} data-testid={contentTestId}>
        <header>
          <h2>{title}</h2>
          {headerAction}
        </header>
        {children}
      </section>
    ) : null,
}));

import { ConversationUsageDisplay } from "./conversation-usage-display";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.touch = true;
  mocks.usage = null;
});

describe("ConversationUsageDisplay", () => {
  it("opens the touch drawer with the latest usage and requests persisted response detail", () => {
    render(<ConversationUsageDisplay taskId="task-1" sessionId="session-1" />);
    const trigger = screen.getByTestId("conversation-usage-trigger");

    expect(trigger.className).toContain("h-11");
    fireEvent.click(trigger);

    expect(screen.getByRole("dialog", { name: "Conversation usage" })).toBeTruthy();
    expect(screen.getByTestId("usage-turn-summary").textContent).toContain("1,200");
    expect(mocks.loadTurnDetail).toHaveBeenCalledWith("turn-1");
    expect(screen.getByRole("button", { name: "View response details" }).className).toContain(
      "min-h-11",
    );
    const closeButtons = screen.getAllByRole("button", { name: "Close" });
    expect(closeButtons.at(-1)?.className).toContain("min-h-11");
  });

  it("keeps the refreshed latest turn ahead of stale loaded detail", () => {
    const makeTurn = (turnId: string, tokens: number): UsageTurn => {
      const breakdown = {
        input_tokens: tokens,
        cached_read_tokens: 0,
        cached_write_tokens: 0,
        output_tokens: 0,
        thought_tokens: 0,
        total_tokens: tokens,
        output_complete: true,
        event_count: 1,
        unpriced_count: 0,
        overflow: false,
      };
      return {
        turn_id: turnId,
        completeness: "exact",
        direct: breakdown,
        child: { ...breakdown, input_tokens: 0, total_tokens: 0, event_count: 0 },
        total: breakdown,
        cost_sources: [],
        last_response: {
          usage_event_id: `usage-${turnId}`,
          input_tokens: tokens,
          cached_read_tokens: 0,
          output_tokens: 0,
          total_tokens: tokens,
          cost_source: "unpriced",
          estimated: false,
          occurred_at: "2026-09-24T00:00:00Z",
        },
      };
    };
    const latestTurn = makeTurn("turn-new", 1200);
    mocks.usage = {
      totals: {
        scope: "session",
        scope_id: "session-1",
        tokens_in: 1200,
        tokens_cached_read: 0,
        tokens_cached_write: 0,
        tokens_out: 0,
        tokens_thought: 0,
        tokens_total: 1200,
        cost_subcents: 0,
        event_count: 2,
        estimated_event_count: 0,
        unpriced_event_count: 2,
        output_tokens_complete: true,
        first_event_at: null,
        last_event_at: null,
      } as SessionUsageTotals,
      latestTurn,
      detail: {
        key: "task-1\u0000session-1",
        turnId: "turn-old",
        turn: makeTurn("turn-old", 800),
        loading: false,
        error: false,
      },
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    };

    render(<ConversationUsageDisplay taskId="task-1" sessionId="session-1" />);
    fireEvent.click(screen.getByTestId("conversation-usage-trigger"));

    const summary = screen.getByTestId("usage-turn-summary").textContent;
    expect(summary).toContain("1,200");
    expect(summary).not.toContain("800");
  });
});
