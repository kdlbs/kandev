import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { WorkflowStepAgentProfileSelector } from "./workflow-step-agent-profile-selector";

const breakpoint = { isMobile: false };
const ARIA_PRESSED = "aria-pressed";
const ARIA_TRUE = "true";
const START_NEW_TEST_ID = "step-1-profile-session-start-new";

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

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => (
    <span data-testid={`agent-logo-${agentName}`} aria-hidden="true" />
  ),
}));

const step = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Review",
  position: 0,
  color: "bg-blue-500",
  agent_profile_id: "profile-a",
  profile_session_start_policy: "reuse",
  profile_session_end_policy: "complete",
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

  it("uses nested lifecycle navigation and updates start and end independently", () => {
    const { onUpdate, trigger } = renderSelector();

    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId("step-1-profile-session-lifecycle-select"));
    expect(screen.getByText("When this step starts:")).toBeTruthy();
    expect(screen.getByText("When this step ends:")).toBeTruthy();
    expect(
      screen.getByTestId("step-1-profile-session-start-reuse").getAttribute(ARIA_PRESSED),
    ).toBe(ARIA_TRUE);
    expect(
      screen.getByTestId("step-1-profile-session-end-complete").getAttribute(ARIA_PRESSED),
    ).toBe(ARIA_TRUE);

    fireEvent.click(screen.getByTestId(START_NEW_TEST_ID));
    fireEvent.click(screen.getByTestId("step-1-profile-session-end-park"));

    expect(onUpdate).toHaveBeenNthCalledWith(1, { profile_session_start_policy: "new" });
    expect(onUpdate).toHaveBeenNthCalledWith(2, { profile_session_end_policy: "park" });
  });

  it("renders the same nested surface in the mobile drawer", () => {
    breakpoint.isMobile = true;
    const { trigger } = renderSelector({
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
    });

    fireEvent.click(trigger);
    expect(screen.getByRole("heading", { name: "Agent Profile" })).toBeTruthy();
    fireEvent.click(screen.getByTestId("step-1-profile-session-lifecycle-select"));
    expect(screen.getByRole("heading", { name: "Session lifecycle" })).toBeTruthy();
    expect(screen.getByTestId(START_NEW_TEST_ID).getAttribute(ARIA_PRESSED)).toBe(ARIA_TRUE);
    expect(screen.getByTestId("step-1-profile-session-end-park").getAttribute(ARIA_PRESSED)).toBe(
      ARIA_TRUE,
    );
  });

  it("keeps the current profile and policy visible while read-only", () => {
    const { trigger } = renderSelector(
      { profile_session_start_policy: "new", profile_session_end_policy: "park" },
      true,
    );

    expect((trigger as HTMLButtonElement).disabled).toBe(true);
    expect(trigger.textContent).toContain("Codex • Fast");
    expect(trigger.textContent).toContain("New on start");
    expect(trigger.textContent).toContain("Park on end");
    expect(screen.getByTestId("agent-logo-codex")).toBeTruthy();
  });

  it("keeps lifecycle editing available when conditional session settings are configured", () => {
    const { onUpdate, trigger } = renderSelector({
      events: { on_enter: [{ type: "configure_session", config: { rules: [] } }] },
    });

    expect((trigger as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(trigger);
    expect(screen.getByTestId("step-1-agent-profile-help")).toBeTruthy();
    expect(
      screen.getByTestId("step-1-profile-option-profile-b").getAttribute("data-disabled"),
    ).toBe("true");

    fireEvent.click(screen.getByTestId("step-1-profile-session-lifecycle-select"));
    expect((screen.getByTestId(START_NEW_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId(START_NEW_TEST_ID));
    expect(onUpdate).toHaveBeenCalledWith({ profile_session_start_policy: "new" });
  });
});
