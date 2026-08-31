import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { WorkflowStepAgentProfileSelector } from "./workflow-step-agent-profile-selector";

const breakpoint = { isMobile: false };

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
}));

vi.mock("@/hooks/domains/settings/use-healthy-agent-profiles", () => ({
  useHealthyAgentProfiles: () => [
    {
      id: "profile-a",
      label: "Codex • Fast",
      agent_id: "agent-a",
      agent_name: "codex",
      cli_passthrough: false,
    },
    {
      id: "profile-b",
      label: "Claude • Review",
      agent_id: "agent-b",
      agent_name: "claude",
      cli_passthrough: false,
    },
  ],
}));

const step = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Review",
  position: 0,
  color: "bg-blue-500",
  agent_profile_id: "profile-a",
  profile_session_policy: "complete",
  created_at: "",
  updated_at: "",
} as WorkflowStep;

beforeEach(() => {
  breakpoint.isMobile = false;
});

afterEach(cleanup);

function renderSelector(overrides: Partial<WorkflowStep> = {}, readOnly = false) {
  const onUpdate = vi.fn();
  const currentStep = { ...step, ...overrides };
  render(
    <WorkflowStepAgentProfileSelector
      step={currentStep}
      savedStep={step}
      onUpdate={onUpdate}
      readOnly={readOnly}
    />,
  );
  return { onUpdate, trigger: screen.getByTestId("step-agent-profile-select") };
}

describe("WorkflowStepAgentProfileSelector", () => {
  it("searches profiles and updates the selected profile from the desktop popover", async () => {
    const { onUpdate, trigger } = renderSelector();

    fireEvent.click(trigger);
    expect(screen.getByPlaceholderText("Search agent profiles...")).toBeTruthy();
    expect(screen.getAllByText("Codex • Fast").length).toBeGreaterThan(1);

    fireEvent.click(screen.getByTestId("step-1-profile-option-profile-b"));

    expect(onUpdate).toHaveBeenCalledWith({ agent_profile_id: "profile-b" });
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("uses nested session behavior navigation and reports the selected policy", () => {
    const { onUpdate, trigger } = renderSelector();

    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId("step-1-profile-session-policy-select"));
    expect(screen.getByText("Park and reuse the previous session")).toBeTruthy();

    fireEvent.click(screen.getByTestId("step-1-profile-session-policy-park_new"));

    expect(onUpdate).toHaveBeenCalledWith({ profile_session_policy: "park_new" });
  });

  it("renders the same nested surface in the mobile drawer", () => {
    breakpoint.isMobile = true;
    const { trigger } = renderSelector({ profile_session_policy: "park_reuse" });

    fireEvent.click(trigger);
    expect(screen.getByRole("heading", { name: "Agent Profile" })).toBeTruthy();
    fireEvent.click(screen.getByTestId("step-1-profile-session-policy-select"));
    expect(
      screen.getByRole("heading", { name: "Session handling when the profile changes" }),
    ).toBeTruthy();
  });

  it("keeps the current profile and policy visible while read-only", () => {
    const { trigger } = renderSelector({ profile_session_policy: "park_reuse" }, true);

    expect((trigger as HTMLButtonElement).disabled).toBe(true);
    expect(trigger.textContent).toContain("Codex • Fast");
    expect(trigger.textContent).toContain("Park and reuse the previous session");
  });

  it("keeps profile switching disabled when conditional session settings are configured", () => {
    const { trigger } = renderSelector({
      events: { on_enter: [{ type: "configure_session", config: { rules: [] } }] },
    });

    expect((trigger as HTMLButtonElement).disabled).toBe(true);
  });
});
