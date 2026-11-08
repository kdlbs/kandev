import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import {
  ClarificationEscapeGuardProvider,
  type ClarificationEscapeGuardEntry,
  type ClarificationEscapeGuardRegistry,
} from "@/hooks/use-clarification-escape-guard";
import { isContextWindowReliable, TokenUsageDisplay } from "./token-usage-display";

const TOOLTIP_ROOT_TESTID = "tooltip-root";
const COPILOT_NATIVE = "copilot_native" as const;
const COMPACTIONS_LABEL = "Compactions";
const COMPACTIONS_ROW_TESTID = "context-window-compactions-row";

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

function withEscapeGuardRegistry(node: React.ReactNode) {
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
    <ClarificationEscapeGuardProvider value={registry}>{node}</ClarificationEscapeGuardProvider>,
  );
  return holder;
}

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

describe("TokenUsageDisplay detail demand", () => {
  it("does not request session updates for a hidden chat", () => {
    vi.mocked(useSessionContextWindow).mockClear().mockReturnValue(undefined);

    render(<TokenUsageDisplay sessionId="sess-1" detailActive={false} />);

    expect(useSessionContextWindow).toHaveBeenCalledWith("sess-1", false);
  });
});

describe("TokenUsageDisplay", () => {
  it("transitions only the context ring arc", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const { container } = render(<TokenUsageDisplay sessionId="sess-1" />);
    const circles = container.querySelectorAll("circle");
    const usageCircle = circles[1];

    expect(usageCircle).toBeDefined();
    expect(usageCircle.getAttribute("class")).toContain("transition-[stroke-dashoffset]");
    expect(usageCircle.getAttribute("class")).not.toContain("transition-all");
  });

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
    const compactionRow = getByTestId(COMPACTIONS_ROW_TESTID);
    expect(compactionRow.textContent).toContain(COMPACTIONS_LABEL);
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
    expect(usage.textContent).toContain("N/A%");
    expect(usage.textContent).toContain("N/A of 200.0K tokens");
    expect(usage.textContent).toContain("Usage data appears after the first completed turn.");
    expect(usage.textContent).not.toContain("0%");
    expect(usage.textContent).not.toContain("0 of");
    expect(usage.textContent).toContain("ACP");
    expect(usage.textContent).toContain(COMPACTIONS_LABEL);
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

  it("claims Escape at dialog capture time while the tooltip is pinned", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 56_047,
      remaining: 143_953,
      efficiency: 28,
      compactionCount: 0,
    });

    const holder = withEscapeGuardRegistry(<TokenUsageDisplay sessionId="sess-1" />);
    const trigger = screen.getByRole("button", { name: "Context window: 28% used" });

    fireEvent.click(trigger);

    expect(holder.entry?.test(new KeyboardEvent("keydown", { key: "Escape" }))).toBe(true);
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

function readCompactionHelp(getByLabelText: (text: string) => HTMLElement): HTMLElement {
  const helpButton = getByLabelText("About inferred compaction count");
  const helpId = helpButton.getAttribute("aria-describedby");
  if (!helpId) throw new Error("Expected compaction help to be described");
  const help = document.getElementById(helpId);
  if (!help) throw new Error("Expected compaction help element");
  return help;
}

function expectProviderNeutralHelp(help: HTMLElement) {
  expect(help.textContent).toContain("Kandev infers this count from context usage drops");
  expect(help.textContent).toContain("Missing samples or provider resets can affect accuracy");
  expect(help.textContent).not.toContain("Copilot");
}

describe("TokenUsageDisplay compaction count", () => {
  it.each([
    { name: "missing reporting metadata", compactionReporting: undefined },
    {
      name: "explicit usage drop reporting",
      compactionReporting: {
        source: "usage_drop" as const,
        availability: "available" as const,
        coverage: "observed" as const,
        revision: 1,
      },
    },
  ])(
    "shows the inferred compaction count and its accuracy disclosure for $name",
    ({ compactionReporting }) => {
      vi.mocked(useSessionContextWindow).mockReturnValue({
        size: 200_000,
        used: 56_047,
        remaining: 143_953,
        efficiency: 28,
        compactionCount: 2,
        compactionReporting,
        source: "acp",
      });

      const { getByLabelText, getByText } = render(<TokenUsageDisplay sessionId="sess-1" />);

      expect(getByText("Compactions")).toBeDefined();
      expect(getByText("2", { selector: "span" })).toBeDefined();
      expect(getByLabelText("About inferred compaction count")).toBeDefined();
      const help = readCompactionHelp(getByLabelText);
      expectProviderNeutralHelp(help);
      expect(help.className).toContain("opacity-0");

      const helpButton = getByLabelText("About inferred compaction count");
      fireEvent.click(helpButton);
      expect(help.className).toContain("opacity-100");
      fireEvent.click(helpButton);
      expect(help.className).toContain("opacity-0");
    },
  );

  it("shows zero compactions for a fresh mixed-context session", () => {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 200_000,
      used: 2_877,
      remaining: 197_123,
      efficiency: 2,
      compactionCount: 0,
      source: "acp",
    });

    const { getByLabelText, getByTestId, getByText } = render(
      <TokenUsageDisplay sessionId="sess-1" />,
    );

    const compactionRow = getByTestId(COMPACTIONS_ROW_TESTID);
    expect(compactionRow.textContent).toContain(COMPACTIONS_LABEL);
    expect(compactionRow.textContent).toContain("0");
    expect(getByText("0", { selector: "span" })).toBeDefined();

    expectProviderNeutralHelp(readCompactionHelp(getByLabelText));
  });
});

describe("TokenUsageDisplay compaction coverage", () => {
  function renderWithReporting(reporting: {
    source: typeof COPILOT_NATIVE;
    availability: "available" | "unavailable";
    coverage: "observed" | "partial";
    revision: number;
  }) {
    vi.mocked(useSessionContextWindow).mockReturnValue({
      size: 272_000,
      used: 12_171,
      remaining: 259_829,
      efficiency: 4,
      compactionCount: 3,
      compactionReporting: reporting,
    });
    const view = render(<TokenUsageDisplay sessionId="sess-1" />);
    fireEvent.click(view.getByRole("button", { name: "Context window: 4% used" }));
    return view;
  }

  it("explains available native counting without a status badge", () => {
    const { getByTestId, queryByTestId } = renderWithReporting({
      source: COPILOT_NATIVE,
      availability: "available",
      coverage: "observed",
      revision: 2,
    });
    expect(queryByTestId("context-window-compactions-status")).toBeNull();
    const row = getByTestId(COMPACTIONS_ROW_TESTID);
    expect(row.textContent).toContain("successful parent compaction events");
    expect(row.textContent).toContain("Historical estimates");
  });

  it("shows partial coverage as sticky missing evidence", () => {
    const { getByTestId } = renderWithReporting({
      source: COPILOT_NATIVE,
      availability: "available",
      coverage: "partial",
      revision: 3,
    });
    expect(getByTestId("context-window-compactions-status").textContent).toBe("Partial");
    expect(getByTestId(COMPACTIONS_ROW_TESTID).textContent).toContain(
      "Some compaction evidence is missing",
    );
  });

  it("shows unavailable collection without claiming zero compactions", () => {
    const { getByTestId } = renderWithReporting({
      source: COPILOT_NATIVE,
      availability: "unavailable",
      coverage: "partial",
      revision: 4,
    });
    expect(getByTestId("context-window-compactions-status").textContent).toBe("Unavailable");
    expect(getByTestId(COMPACTIONS_ROW_TESTID).textContent).toContain("zero does not prove");
  });
});
