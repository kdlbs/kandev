import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
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
    defaultMutationPendingId,
    onKindChange,
    onToggleSavedDefault,
    savedPresets,
  }: {
    defaultMutationPendingId?: string | null;
    onKindChange?: (kind: "pr" | "issue") => void;
    onToggleSavedDefault?: (id: string) => void;
    savedPresets: Array<{ id: string }>;
  }) => {
    const initialDefaultAction = useRef(onToggleSavedDefault);
    return (
      <div
        data-testid="scope-bar-default-action"
        data-pending={defaultMutationPendingId !== null}
        data-pending-id={defaultMutationPendingId}
        data-callback-stable={initialDefaultAction.current === onToggleSavedDefault}
      >
        <button
          type="button"
          data-testid="scope-bar-default-toggle"
          onClick={() => onToggleSavedDefault?.(savedPresets[0]?.id ?? "missing-preset")}
        />
        <button
          type="button"
          data-testid="scope-bar-kind-switch"
          onClick={() => onKindChange?.("issue")}
        />
        <button
          type="button"
          data-testid="scope-bar-active-kind"
          onClick={() => onKindChange?.("pr")}
        />
      </div>
    );
  },
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function expectExplicitKindSwitchRequest() {
  const onSelect = vi.fn();

  render(
    <PresetsScopeBar
      selected={{ kind: "pr", source: "preset", id: "review" }}
      onSelect={onSelect}
      savedPresets={[]}
      onDeleteSaved={vi.fn()}
      canSaveCurrent={false}
      onSaveCurrent={vi.fn()}
      onToggleSavedDefault={vi.fn()}
      defaultMutationPendingId={null}
      prPresets={[]}
      issuePresets={[]}
    />,
  );

  fireEvent.click(screen.getByTestId("scope-bar-kind-switch"));
  expect(onSelect).toHaveBeenCalledWith({ kind: "issue", source: "kind-switch" });
}

function expectActiveKindRequestIgnored() {
  const onSelect = vi.fn();

  render(
    <PresetsScopeBar
      selected={{ kind: "pr", source: "preset", id: "review" }}
      onSelect={onSelect}
      savedPresets={[]}
      onDeleteSaved={vi.fn()}
      canSaveCurrent={false}
      onSaveCurrent={vi.fn()}
      onToggleSavedDefault={vi.fn()}
      defaultMutationPendingId={null}
      prPresets={[]}
      issuePresets={[]}
    />,
  );

  fireEvent.click(screen.getByTestId("scope-bar-active-kind"));
  expect(onSelect).not.toHaveBeenCalled();
}

describe("PresetsScopeBar default adapter", () => {
  it("emits an explicit kind-switch request", expectExplicitKindSwitchRequest);

  it("ignores an active-kind request at the wrapper boundary", expectActiveKindRequestIgnored);

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
        defaultMutationPendingId={savedPreset.id}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    expect(screen.getByTestId("scope-bar-default-action").getAttribute("data-pending")).toBe(
      "true",
    );
    expect(screen.getByTestId("scope-bar-default-action").getAttribute("data-pending-id")).toBe(
      savedPreset.id,
    );
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
        defaultMutationPendingId={null}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    fireEvent.click(screen.getByTestId("scope-bar-default-toggle"));
    expect(onToggleSavedDefault).toHaveBeenCalledWith(savedPreset);
  });

  it("keeps the default adapter stable while using refreshed presets", () => {
    const savedPresets = [savedPreset];
    const props = {
      selected: { kind: "pr", source: "saved", id: savedPreset.id } as const,
      onSelect: vi.fn(),
      savedPresets,
      onDeleteSaved: vi.fn(),
      canSaveCurrent: false,
      onSaveCurrent: vi.fn(),
      onToggleSavedDefault: vi.fn(),
      defaultMutationPendingId: null,
      prPresets: [],
      issuePresets: [],
    };
    const { rerender } = render(<PresetsScopeBar {...props} />);
    const refreshedPreset = { ...savedPreset, label: "Updated", isDefault: true };

    rerender(<PresetsScopeBar {...props} savedPresets={[refreshedPreset]} />);

    expect(
      screen.getByTestId("scope-bar-default-action").getAttribute("data-callback-stable"),
    ).toBe("true");
    fireEvent.click(screen.getByTestId("scope-bar-default-toggle"));
    expect(props.onToggleSavedDefault).toHaveBeenCalledWith(refreshedPreset);
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
        defaultMutationPendingId={null}
        prPresets={[]}
        issuePresets={[]}
      />,
    );

    fireEvent.click(screen.getByTestId("scope-bar-default-toggle"));
    expect(warn).toHaveBeenCalledWith("[github:presets] default toggle target missing", {
      id: "missing-preset",
    });
    expect(onToggleSavedDefault).not.toHaveBeenCalled();
  });
});
