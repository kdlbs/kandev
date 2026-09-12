import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SavedPreset } from "./saved-preset-model";
import { PresetsSidebar } from "./presets-sidebar";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

const savedPreset: SavedPreset = {
  id: "saved-pr",
  kind: "pr",
  label: "Kandev PRs",
  customQuery: "author:@me is:open",
  repoFilter: "kdlbs/kandev",
  createdAt: "2026-08-10T00:00:00Z",
  isDefault: false,
};

async function expectDeleteDoesNotSelect(phone = false) {
  if (phone) Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
  const onSelect = vi.fn();
  const onDeleteSaved = vi.fn();
  render(
    <PresetsSidebar
      selected={{ kind: "pr", source: "saved", id: savedPreset.id }}
      onSelect={onSelect}
      savedPresets={[savedPreset]}
      onDeleteSaved={onDeleteSaved}
      canSaveCurrent={false}
      onSaveCurrent={vi.fn()}
      onToggleSavedDefault={vi.fn()}
      defaultMutationPendingId={null}
      prPresets={[]}
      issuePresets={[]}
    />,
  );

  const trigger = screen.getByRole("button", { name: "Delete Kandev PRs saved query" });
  fireEvent.click(trigger);
  if (phone) {
    expect(trigger.isConnected).toBe(true);
    expect(
      screen.getByRole("dialog", { name: "Delete Kandev PRs?" }).getAttribute("data-slot"),
    ).toBe("drawer-content");
  }

  expect(onDeleteSaved).not.toHaveBeenCalled();
  expect(onSelect).not.toHaveBeenCalled();
  const confirmation = screen.getByRole("group", { name: "Delete Kandev PRs?" });
  fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
  expect(onDeleteSaved).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Delete Kandev PRs saved query" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete Kandev PRs" }));

  await waitFor(() => expect(onDeleteSaved).toHaveBeenCalledWith(savedPreset.id));
  expect(onDeleteSaved).toHaveBeenCalledOnce();
  expect(onSelect).not.toHaveBeenCalled();
}

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
        defaultMutationPendingId={savedPreset.id}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    expect(
      (
        screen.getByRole("button", {
          name: "Updating default view… Delete Kandev PRs saved query",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    const defaultAction = screen.getByRole("button", {
      name: "Updating default view… Set Kandev PRs as default view",
    }) as HTMLButtonElement;
    expect(defaultAction.disabled).toBe(true);
    expect(defaultAction.getAttribute("aria-busy")).toBeNull();
    expect(defaultAction.querySelector("svg")?.getAttribute("class")).toContain("animate-pulse");
  });

  it("sends only the destination kind when switching views", () => {
    const onSelect = vi.fn();
    render(
      <PresetsSidebar
        selected={{ kind: "pr", source: "saved", id: savedPreset.id }}
        onSelect={onSelect}
        savedPresets={[savedPreset]}
        onDeleteSaved={vi.fn()}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        onToggleSavedDefault={vi.fn()}
        defaultMutationPendingId={null}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Issues" }));

    expect(onSelect).toHaveBeenCalledWith({ kind: "issue", source: "kind-switch" });
  });

  it("ignores reselecting the active kind", () => {
    const onSelect = vi.fn();
    render(
      <PresetsSidebar
        selected={{ kind: "pr", source: "saved", id: savedPreset.id }}
        onSelect={onSelect}
        savedPresets={[savedPreset]}
        onDeleteSaved={vi.fn()}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        onToggleSavedDefault={vi.fn()}
        defaultMutationPendingId={null}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Pull requests" }));

    expect(onSelect).not.toHaveBeenCalled();
  });

  it("deletes a saved query without selecting it", () => expectDeleteDoesNotSelect());
  it("keeps a phone saved query row mounted during confirmation", () =>
    expectDeleteDoesNotSelect(true));
});
