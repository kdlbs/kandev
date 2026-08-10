import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PresetsScopeBar } from "./presets-scope-bar";

vi.mock("@/components/integrations/presets-scope-bar-base", () => ({
  IntegrationScopeBar: ({
    onToggleSavedDefault,
  }: {
    onToggleSavedDefault?: (id: string) => void;
  }) => {
    onToggleSavedDefault?.("missing-preset");
    return null;
  },
}));

afterEach(() => {
  vi.restoreAllMocks();
});

describe("PresetsScopeBar default adapter", () => {
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
