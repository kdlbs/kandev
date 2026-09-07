import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { SavedPreset } from "./use-saved-presets";
import { PresetsSidebar } from "./presets-sidebar";

const SAVED: SavedPreset = {
  id: "saved-mr",
  kind: "mr",
  label: "Ready MRs",
  customQuery: "state=opened",
  projectFilter: "kdlbs/kandev",
  milestone: "",
  preset: "",
  createdAt: "2026-09-07T00:00:00Z",
};

describe("GitLab PresetsSidebar", () => {
  afterEach(cleanup);

  it("uses visible touch controls and deletes without selecting only after confirmation", async () => {
    const onDeleteSaved = vi.fn();
    const onSelect = vi.fn();
    render(
      <PresetsSidebar
        selected={{ kind: "mr", source: "saved", id: SAVED.id }}
        onSelect={onSelect}
        savedPresets={[SAVED]}
        onDeleteSaved={onDeleteSaved}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        mrPresets={[]}
        issuePresets={[]}
      />,
    );

    const deleteButton = screen.getByRole("button", { name: "Delete Ready MRs saved query" });
    expect(deleteButton.className).toContain("h-12");
    expect(deleteButton.className).not.toContain("opacity-0");
    fireEvent.click(deleteButton);

    expect(onDeleteSaved).not.toHaveBeenCalled();
    expect(onSelect).not.toHaveBeenCalled();
    const confirmation = screen.getByRole("group", { name: "Delete Ready MRs?" });
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    fireEvent.click(screen.getByRole("button", { name: "Delete Ready MRs saved query" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete Ready MRs" }));

    await waitFor(() => expect(onDeleteSaved).toHaveBeenCalledWith(SAVED.id));
    expect(onDeleteSaved).toHaveBeenCalledOnce();
    expect(onSelect).not.toHaveBeenCalled();
  });
});
