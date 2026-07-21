import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
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
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Tooltip: ({ children, open }: { children: React.ReactNode; open?: boolean }) => (
    <div data-testid="tooltip-root" data-open={open}>
      {children}
    </div>
  ),
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

  it("opens the tooltip when the context ring is tapped", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: "Context window: 28% used" }));

    expect(getByTestId("tooltip-root").getAttribute("data-open")).toBe("true");
  });

  it("closes a tapped tooltip when Escape is pressed", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: "Context window: 28% used" }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(getByTestId("tooltip-root").getAttribute("data-open")).toBe("false");
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

    const { container, getByRole, getByTestId, getByText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    expect(container.querySelector('[data-testid="doughnut-subscription-usage"]')).not.toBeNull();
    expect(getByRole("button", { name: "Context window: 28% used" })).toBeDefined();
    expect(getByText("Context window")).toBeDefined();
    expect(getByText("56.0K of 200.0K tokens")).toBeDefined();
    expect(getByText(/Subscription · max/i)).toBeDefined();
    expect(
      getByTestId("context-window-usage").compareDocumentPosition(
        getByTestId("doughnut-subscription-usage"),
      ) & Node.DOCUMENT_POSITION_FOLLOWING,
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

describe("TokenUsageDisplay context source", () => {
  it("labels agent-reported ACP data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      source: "acp",
    });

    const { container, getAllByTestId, getByText, getByLabelText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    const tokenCount = getByText("56.0K of 200.0K tokens");
    const source = getByText("ACP");

    expect(container.querySelector(".cursor-help")).not.toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
    expect(tokenCount.closest('[data-testid="context-window-token-row"]')).toBe(
      source.closest('[data-testid="context-window-token-row"]'),
    );
    expect(getAllByTestId("tooltip-root")).toHaveLength(1);
    expect(getByLabelText("About context window source")).toBeDefined();
    expect(getByText(/ACP is the active session's effective window/i)).toBeDefined();
  });

  it("labels model API fallback data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 128_000,
      used: 64_000,
      remaining: 64_000,
      efficiency: 50,
      source: "api",
    });

    const { getByText, getByLabelText } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(getByText("API")).toBeDefined();
    expect(getByLabelText("About context window source")).toBeDefined();
    expect(getByText(/model's advertised maximum from the catalogue/i)).toBeDefined();
  });
});
