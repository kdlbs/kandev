import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { isContextWindowReliable, TokenUsageDisplay } from "./token-usage-display";

vi.mock("@/hooks/domains/session/use-session-context-window", () => ({
  useSessionContextWindow: vi.fn(),
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
      remaining: 0,
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
});
