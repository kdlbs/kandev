import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SavedPreset } from "./saved-preset-model";
import { PresetsScopeBar } from "./presets-scope-bar";

const savedPreset: SavedPreset = {
  id: "saved-pr",
  kind: "pr",
  label: "Needs review",
  customQuery: "review-requested:@me is:open",
  repoFilter: "kdlbs/kandev",
  createdAt: "2026-08-10T00:00:00Z",
  isDefault: false,
};

vi.mock("@/components/integrations/presets-scope-bar-base", () => ({
  IntegrationScopeBar: ({
    defaultMutationPending,
    onToggleSavedDefault,
    savedPresets,
  }: {
    defaultMutationPending?: boolean;
    onToggleSavedDefault?: (id: string) => void;
    savedPresets: Array<{ id: string }>;
  }) => {
    onToggleSavedDefault?.(savedPresets[0]?.id ?? "missing-preset");
    return <div data-testid="scope-bar-default-action" aria-busy={defaultMutationPending} />;
  },
}));

afterEach(() => {
  vi.restoreAllMocks();
});

describe("PresetsScopeBar default adapter", () => {
  it("forwards pending state to the desktop default action", () => {
    render(
      <PresetsScopeBar
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

    expect(screen.getByTestId("scope-bar-default-action").getAttribute("aria-busy")).toBe("true");
  });

  it("forwards a matching saved preset to the domain callback", () => {
    const onToggleSavedDefault = vi.fn();

    render(
      <PresetsScopeBar
        selected={{ kind: "pr", source: "saved", id: savedPreset.id }}
        onSelect={vi.fn()}
        savedPresets={[savedPreset]}
        onDeleteSaved={vi.fn()}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        onToggleSavedDefault={onToggleSavedDefault}
        defaultMutationPending={false}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    expect(onToggleSavedDefault).toHaveBeenCalledWith(savedPreset);
  });

  it("warns in development when a stale default target is requested", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const onToggleSavedDefault = vi.fn();

    render(
      <PresetsScopeBar
        selected={{ kind: "pr", source: "preset", id: "review" }}
        onSelect={vi.fn()}
        savedPresets={[]}
        onDeleteSaved={vi.fn()}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
        onToggleSavedDefault={onToggleSavedDefault}
        defaultMutationPending={false}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    expect(warn).toHaveBeenCalledWith("[github:presets] default toggle target missing", {
      id: "missing-preset",
    });
    expect(onToggleSavedDefault).not.toHaveBeenCalled();
  });
});
