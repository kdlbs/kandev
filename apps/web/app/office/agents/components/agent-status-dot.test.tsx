import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AgentStatusDot } from "./agent-status-dot";

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => undefined,
}));

afterEach(cleanup);

describe("AgentStatusDot", () => {
  it("uses compositor opacity motion while the agent is working", () => {
    const { container } = render(<AgentStatusDot status="working" />);

    const dot = container.firstElementChild as HTMLElement;
    expect(dot.hasAttribute("data-compositor-pulse")).toBe(true);
    expect(dot.className).toContain("animate-pulse");
    expect(dot.getAttribute("title")).toBe("working");
  });

  it("keeps settled agent states static", () => {
    const { container } = render(<AgentStatusDot status="idle" />);

    const dot = container.firstElementChild as HTMLElement;
    expect(dot.hasAttribute("data-compositor-pulse")).toBe(false);
    expect(dot.className).not.toContain("animate-pulse");
  });
});
