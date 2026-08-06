import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProfileListItem } from "./profile-list-item";
import type { Agent, AgentProfile } from "@/lib/types/http";

const agent = {
  id: "a1",
  name: "claude-acp",
  supports_mcp: false,
  profiles: [{ agentDisplayName: "Claude Code" }],
} as unknown as Agent;

const profile = { id: "p1", name: "default", enabled: true } as unknown as AgentProfile;

afterEach(cleanup);

describe("ProfileListItem", () => {
  it("renders the profile row with a switch reflecting enabled state", () => {
    render(<ProfileListItem agent={agent} profile={profile} onToggleEnabled={vi.fn()} />);
    const toggle = screen.getByTestId("profile-enabled-toggle-p1");
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(toggle.getAttribute("aria-label")).toBe("Disable default");
  });

  it("shows a disabled profile with the switch off", () => {
    render(
      <ProfileListItem
        agent={agent}
        profile={{ ...profile, enabled: false }}
        onToggleEnabled={vi.fn()}
      />,
    );
    const toggle = screen.getByTestId("profile-enabled-toggle-p1");
    expect(toggle.getAttribute("data-state")).toBe("unchecked");
    expect(toggle.getAttribute("aria-label")).toBe("Enable default");
  });

  it("calls onToggleEnabled with the profile and the next value on click", () => {
    const onToggle = vi.fn();
    render(<ProfileListItem agent={agent} profile={profile} onToggleEnabled={onToggle} />);
    fireEvent.click(screen.getByTestId("profile-enabled-toggle-p1"));
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle).toHaveBeenCalledWith(profile, false);
  });
});
