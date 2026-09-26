import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SettingsMenuMode } from "@/lib/settings/settings-menu-mode";

const state = {
  settingsMenu: {
    mode: "persistent" as SettingsMenuMode,
    savedMode: "persistent" as SettingsMenuMode,
    expandedKeys: ["row:/settings/workspaces", "workspace:ws-1"] as string[],
  },
  setSettingsMenuExpandedKeys: vi.fn((keys: string[]) => {
    state.settingsMenu.expandedKeys = keys;
  }),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

import { CollapseAllButton } from "./collapse-all-button";

const LABEL = "Collapse all";

// The app wraps the whole shell in one (app/layout.tsx); Radix needs it present.
function renderButton() {
  return render(
    <TooltipProvider>
      <CollapseAllButton />
    </TooltipProvider>,
  );
}

describe("CollapseAllButton", () => {
  beforeEach(() => {
    state.settingsMenu.mode = "persistent";
    state.settingsMenu.expandedKeys = ["row:/settings/workspaces", "workspace:ws-1"];
    state.setSettingsMenuExpandedKeys.mockClear();
  });

  afterEach(cleanup);

  it("shuts every open branch", () => {
    renderButton();

    fireEvent.click(screen.getByRole("button", { name: LABEL }));

    expect(state.setSettingsMenuExpandedKeys).toHaveBeenCalledWith([]);
  });

  it("greys out rather than vanishing when nothing is open", () => {
    state.settingsMenu.expandedKeys = [];
    renderButton();

    // Present but inert: a control that disappears is harder to find again
    // than one that dims.
    expect(screen.getByRole("button", { name: LABEL }).hasAttribute("disabled")).toBe(true);
  });

  for (const mode of ["flat", "accordion"] as const) {
    it(`stays out of ${mode} mode, which cannot accumulate open branches`, () => {
      state.settingsMenu.mode = mode;
      renderButton();

      expect(screen.queryByRole("button", { name: LABEL })).toBeNull();
    });
  }
});
