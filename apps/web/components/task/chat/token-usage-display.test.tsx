import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { isContextWindowReliable, TokenUsageDisplay } from "./token-usage-display";

const TOOLTIP_ROOT_TESTID = "tooltip-root";

vi.mock("@/hooks/domains/session/use-session-context-window", () => ({
  useSessionContextWindow: vi.fn(),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Tooltip: ({ children, open }: { children: React.ReactNode; open?: boolean }) => (
    <div data-testid={TOOLTIP_ROOT_TESTID} data-open={open}>
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
      compactionCount: 0,
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
      compactionCount: 0,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: "Context window: 28% used" }));

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("true");
    const compactionRow = getByTestId("context-window-compactions-row");
    expect(compactionRow.textContent).toContain("Compactions");
    expect(compactionRow.textContent).toContain("0");
  });

  it("renders an unmeasured pending state when usage is zero", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 0,
      remaining: 200_000,
      efficiency: 0,
      compactionCount: 2,
      source: "acp",
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    const trigger = getByRole("button", { name: "Context window: usage not measured" });
    fireEvent.click(trigger);

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("true");
    const usage = getByTestId("context-window-usage");
    expect(usage.textContent).toContain("—%");
    expect(usage.textContent).toContain("— of 200.0K tokens");
    expect(usage.textContent).toContain("Usage data appears after the first completed turn.");
    expect(usage.textContent).not.toContain("0%");
    expect(usage.textContent).not.toContain("0 of");
    expect(usage.textContent).toContain("ACP");
    expect(usage.textContent).toContain("Compactions");
  });

  it("closes a tapped tooltip when Escape is pressed", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const { getByRole, getByTestId } = render(<TokenUsageDisplay sessionId="sess-1" />);

    fireEvent.click(getByRole("button", { name: "Context window: 28% used" }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(getByTestId(TOOLTIP_ROOT_TESTID).getAttribute("data-open")).toBe("false");
  });
});

describe("TokenUsageDisplay context source", () => {
  it("labels agent-reported ACP data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 2,
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
    expect(getAllByTestId(TOOLTIP_ROOT_TESTID)).toHaveLength(1);
    expect(getByLabelText("About context window source")).toBeDefined();
    expect(getByText(/ACP is the active session's effective window/i)).toBeDefined();
  });

  it("shows the inferred compaction count and its accuracy disclosure", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 2,
      source: "acp",
    });

    const { container, getByLabelText, getByText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    expect(getByText("Compactions")).toBeDefined();
    expect(getByText("2", { selector: "span" })).toBeDefined();
    expect(getByLabelText("About inferred compaction count")).toBeDefined();
    expect(container.textContent).toContain(
      "Kandev infers this count from observed context usage drops",
    );

    const helpButton = getByLabelText("About inferred compaction count");
    const helpId = helpButton.getAttribute("aria-describedby");
    if (!helpId) throw new Error("Expected compaction help to be described");
    const help = document.getElementById(helpId);
    if (!help) throw new Error("Expected compaction help element");
    expect(help.className).toContain("opacity-0");

    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-100");
    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-0");
  });

  it("labels model API fallback data", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 128_000,
      used: 64_000,
      remaining: 64_000,
      efficiency: 50,
      compactionCount: 0,
      source: "api",
    });

    const { getByText, getByLabelText } = render(<TokenUsageDisplay sessionId="sess-1" />);

    expect(getByText("API")).toBeDefined();
    const helpButton = getByLabelText("About context window source");
    const helpId = helpButton.getAttribute("aria-describedby");
    if (!helpId) throw new Error("Expected source help to be described");
    const help = document.getElementById(helpId);
    if (!help) throw new Error("Expected source help element");

    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-100");
    fireEvent.click(helpButton);
    expect(help.className).toContain("opacity-0");
    expect(getByText(/model's advertised maximum from the catalogue/i)).toBeDefined();
  });
});
