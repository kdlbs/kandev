import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SavedPreset } from "./saved-preset-model";
import { PresetsSidebar } from "./presets-sidebar";

afterEach(cleanup);

const savedPreset: SavedPreset = {
  id: "saved-pr",
  kind: "pr",
  label: "Kandev PRs",
  customQuery: "author:@me is:open",
  repoFilter: "kdlbs/kandev",
  createdAt: "2026-08-10T00:00:00Z",
  isDefault: false,
};

describe("PresetsSidebar saved defaults", () => {
  it("disables saved-query actions while a default mutation is pending", () => {
    render(
      <PresetsSidebar
        selected={{ kind: "pr", source: "saved", id: savedPreset.id }}
        onSelect={vi.fn()}
        savedPresets={[savedPreset]}
        onDeleteSaved={vi.fn()}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        onToggleSavedDefault={vi.fn()}
        defaultMutationPending
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    expect(
      (
        screen.getByRole("button", {
          name: "Delete Kandev PRs saved query",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    const defaultAction = screen.getByRole("button", {
      name: "Set Kandev PRs as default view",
    }) as HTMLButtonElement;
    expect(defaultAction.disabled).toBe(true);
    expect(defaultAction.getAttribute("aria-busy")).toBe("true");
    expect(defaultAction.querySelector("svg")?.getAttribute("class")).toContain("animate-pulse");
  });
});
