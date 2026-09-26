import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HideDisabledAgentProfilesSetting } from "./hide-disabled-agent-profiles-setting";

const mocks = vi.hoisted(() => ({
  hideDisabled: false,
  setHideDisabled: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav", () => ({
  useHideDisabledAgentProfilesInNav: () => ({
    hideDisabled: mocks.hideDisabled,
    setHideDisabled: mocks.setHideDisabled,
  }),
}));

const LABEL = "Hide disabled agent profiles from left panel navigation";

function ariaChecked(element: Element | null) {
  return element?.getAttribute("aria-checked");
}

describe("HideDisabledAgentProfilesSetting", () => {
  beforeEach(() => {
    mocks.hideDisabled = false;
    mocks.setHideDisabled.mockClear();
  });
  afterEach(cleanup);

  it("renders the label, description, and an off switch by default", () => {
    render(<HideDisabledAgentProfilesSetting />);

    expect(screen.getByText(LABEL)).toBeTruthy();
    expect(
      screen.getByText(
        "When on, a disabled agent profile is removed from the left panel navigation even if it's still configured.",
      ),
    ).toBeTruthy();
    const switchEl = screen.getByRole("switch", { name: LABEL });
    expect(ariaChecked(switchEl)).toBe("false");
  });

  it("reflects the stored hideDisabled value", () => {
    mocks.hideDisabled = true;
    render(<HideDisabledAgentProfilesSetting />);

    expect(ariaChecked(screen.getByRole("switch", { name: LABEL }))).toBe("true");
  });

  it("saves immediately when toggled", () => {
    render(<HideDisabledAgentProfilesSetting />);

    fireEvent.click(screen.getByRole("switch", { name: LABEL }));

    expect(mocks.setHideDisabled).toHaveBeenCalledWith(true);
  });
});
