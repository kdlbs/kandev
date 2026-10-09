import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickChatAgentPicker } from "./quick-chat-agent-picker";

const responsive = vi.hoisted(() => ({ isMobile: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

afterEach(() => cleanup());

describe("QuickChatAgentPicker phone trigger", () => {
  it("shows the selected trigger label and exposes the selection and help accessibly", () => {
    const options = [
      {
        value: "profile-a",
        label: "Profile A",
        renderLabel: () => <span>Profile A option</span>,
        renderTriggerLabel: () => <span>Profile A with limited tools</span>,
      },
    ];
    render(
      <div>
        <h3 id="agent-label">Agent profile</h3>
        <QuickChatAgentPicker
          options={options}
          value="profile-a"
          onValueChange={vi.fn()}
          disabled={false}
          placeholder="Choose an agent"
          ariaDescribedBy="agent-help"
          labelId="agent-label"
        />
        <p id="agent-help">Select an enabled agent profile.</p>
      </div>,
    );

    const trigger = screen.getByRole("combobox", {
      name: "Agent profile Profile A with limited tools",
    });
    expect(trigger.textContent).toContain("Profile A with limited tools");
    expect(trigger.getAttribute("aria-describedby")).toBe("agent-help");
  });
});
