import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { LiveAgentIndicator } from "./live-agent-indicator";

afterEach(() => {
  cleanup();
});

describe("LiveAgentIndicator", () => {
  it("renders nothing when count is 0", () => {
    const { container } = render(<LiveAgentIndicator count={0} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when count is negative", () => {
    const { container } = render(<LiveAgentIndicator count={-1} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the pulsing dot + '1 live' label when count is 1", () => {
    render(<LiveAgentIndicator count={1} />);
    expect(screen.getByText("1 live")).toBeTruthy();
    const wrapper = screen.getByLabelText("1 active session");
    expect(wrapper).toBeTruthy();
    expect(wrapper.querySelector(".animate-ping")).toBeTruthy();
  });

  it("renders '5 live' when count is 5 and pluralizes the aria-label", () => {
    render(<LiveAgentIndicator count={5} />);
    expect(screen.getByText("5 live")).toBeTruthy();
    expect(screen.getByLabelText("5 active sessions")).toBeTruthy();
  });
});
