import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AgentProfilePicker } from "./agent-profile-picker";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

afterEach(() => cleanup());

const TEST_ID = "agent-profile-picker";
const PROFILE_ONE_ID = "p1";
const PASSTHROUGH_LABEL = "Passthrough profile";

function profile(overrides: Partial<AgentProfileOption> = {}): AgentProfileOption {
  return {
    id: PROFILE_ONE_ID,
    label: "Profile One",
    agent_id: "agent-1",
    agent_name: "Agent One",
    cli_passthrough: false,
    ...overrides,
  };
}

describe("AgentProfilePicker disabledOptionReason", () => {
  it("leaves every option selectable when disabledOptionReason is not passed", () => {
    render(
      <TooltipProvider>
        <AgentProfilePicker
          profiles={[profile({ id: PROFILE_ONE_ID }), profile({ id: "p2", label: "Profile Two" })]}
          value=""
          onValueChange={vi.fn()}
          testId={TEST_ID}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId(TEST_ID));
    for (const option of screen.getAllByRole("option")) {
      expect(option.getAttribute("aria-disabled")).toBe("false");
    }
  });

  it("marks a passthrough profile disabled with its reason while keeping it listed", () => {
    const onValueChange = vi.fn();
    render(
      <TooltipProvider>
        <AgentProfilePicker
          profiles={[
            profile({ id: PROFILE_ONE_ID, label: PASSTHROUGH_LABEL, cli_passthrough: true }),
            profile({ id: "p2", label: "Regular profile" }),
          ]}
          value=""
          onValueChange={onValueChange}
          testId={TEST_ID}
          disabledOptionReason={(p) => (p.cli_passthrough ? "Uses CLI passthrough" : undefined)}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId(TEST_ID));
    const options = screen.getAllByRole("option");
    const labels = options.map((option) => option.textContent?.trim());
    expect(labels).toEqual(expect.arrayContaining([PASSTHROUGH_LABEL, "Regular profile"]));

    const passthroughOption = options.find((o) => o.textContent?.includes(PASSTHROUGH_LABEL))!;
    const regularOption = options.find((o) => o.textContent?.includes("Regular profile"))!;
    expect(passthroughOption.getAttribute("aria-disabled")).toBe("true");
    expect(regularOption.getAttribute("aria-disabled")).toBe("false");

    fireEvent.click(passthroughOption);
    expect(onValueChange).not.toHaveBeenCalled();
    fireEvent.click(regularOption);
    expect(onValueChange).toHaveBeenCalledWith("p2");
  });

  it("shows a stored disabled profile as the current selection instead of an unavailable entry", () => {
    render(
      <TooltipProvider>
        <AgentProfilePicker
          profiles={[
            profile({ id: PROFILE_ONE_ID, label: PASSTHROUGH_LABEL, cli_passthrough: true }),
          ]}
          value={PROFILE_ONE_ID}
          onValueChange={vi.fn()}
          testId={TEST_ID}
          disabledOptionReason={(p) => (p.cli_passthrough ? "Uses CLI passthrough" : undefined)}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(TEST_ID).textContent).toContain(PASSTHROUGH_LABEL);
    fireEvent.click(screen.getByTestId(TEST_ID));
    expect(screen.getAllByRole("option")).toHaveLength(1);
  });
});

describe("AgentProfilePicker disabled", () => {
  it("disables the trigger and blocks selection when disabled is true (AC-004.6)", () => {
    const onValueChange = vi.fn();
    render(
      <TooltipProvider>
        <AgentProfilePicker
          profiles={[profile({ id: PROFILE_ONE_ID })]}
          value={PROFILE_ONE_ID}
          onValueChange={onValueChange}
          testId={TEST_ID}
          disabled
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId(TEST_ID);
    expect(trigger.hasAttribute("disabled")).toBe(true);
    fireEvent.click(trigger);
    expect(screen.queryByRole("option")).toBeNull();
  });

  it("stays interactive when disabled is not passed", () => {
    render(
      <TooltipProvider>
        <AgentProfilePicker
          profiles={[profile({ id: PROFILE_ONE_ID })]}
          value=""
          onValueChange={vi.fn()}
          testId={TEST_ID}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(TEST_ID).hasAttribute("disabled")).toBe(false);
  });
});
