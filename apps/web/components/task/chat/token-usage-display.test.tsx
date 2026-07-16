import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { useSessionAgentUsage } from "@/hooks/domains/session/use-session-agent-usage";
import { isContextWindowReliable, TokenUsageDisplay } from "./token-usage-display";

vi.mock("@/hooks/domains/session/use-session-context-window", () => ({
  useSessionContextWindow: vi.fn(),
}));

vi.mock("@/hooks/domains/session/use-session-agent-usage", () => ({
  useSessionAgentUsage: vi.fn(() => null),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("isContextWindowReliable", () => {
  it("accepts normal usage under the window", () => {
    expect(isContextWindowReliable(200_000, 56_047)).toBe(true);
  });

  it("accepts exactly-full context (100%)", () => {
    expect(isContextWindowReliable(200_000, 200_000)).toBe(true);
  });

  it("rejects impossible usage (used > size) — the wrong-window bug", () => {
    expect(isContextWindowReliable(200_000, 233_900)).toBe(false);
  });

  it("rejects a zero/absent window size", () => {
    expect(isContextWindowReliable(0, 0)).toBe(false);
  });

  it("accepts a correct large window", () => {
    expect(isContextWindowReliable(1_000_000, 233_900)).toBe(true);
  });
});

describe("TokenUsageDisplay", () => {
  it("renders nothing when used exceeds size (wrong-window bug)", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 233_900,
      remaining: -33_900,
      efficiency: 117,
    });

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(container.firstChild).toBeNull();
  });

  it("renders the indicator for valid usage under the window", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
    });

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(container.querySelector(".cursor-help")).not.toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
  });

  it("shows subscription usage rows in the tooltip when the agent has them", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
    });
    vi.mocked(useSessionAgentUsage).mockReturnValue({
      agent_id: "claude-acp",
      display_name: "Claude",
      usage: {
        provider: "anthropic",
        plan: "max",
        windows: [
          {
            label: "5-hour",
            utilization_pct: 86,
            reset_at: new Date(Date.now() + 3 * 3_600_000).toISOString(),
          },
          {
            label: "7-day",
            utilization_pct: 19,
            reset_at: new Date(Date.now() + 30 * 3_600_000).toISOString(),
          },
        ],
        fetched_at: new Date().toISOString(),
      },
    });

    const { container, getByTestId, getByText } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(container.querySelector('[data-testid="doughnut-subscription-usage"]')).not.toBeNull();
    expect(getByText(/Subscription usage · max/i)).toBeDefined();
    expect(
      getByTestId("doughnut-subscription-usage").compareDocumentPosition(getByText(/^28%/)) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // Worst window is 86% → provider status "High".
    expect(getByText("High")).toBeDefined();
    expect(getByText("5h")).toBeDefined();
    expect(getByText("86%")).toBeDefined();
    expect(getByText("7d")).toBeDefined();
    // Reset countdown column (≈3h out) and colored bars per window.
    expect(getByText(/^(3h 0m|2h 59m)$/)).toBeDefined();
    expect(container.querySelector(".bg-amber-500")).not.toBeNull(); // 86% bar
    expect(container.querySelector(".bg-emerald-500")).not.toBeNull(); // 19% bar
  });

  it("omits the subscription block when the agent has no usage", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
    });
    vi.mocked(useSessionAgentUsage).mockReturnValue(null);

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(container.querySelector('[data-testid="doughnut-subscription-usage"]')).toBeNull();
  });
});
