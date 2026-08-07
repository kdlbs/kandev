import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationScopeBar, type ScopePreset } from "./presets-scope-bar-base";

afterEach(cleanup);

describe("IntegrationScopeBar", () => {
  it("uses a host icon when a plugin preset does not provide one", () => {
    const presets = [{ value: "open", label: "Open", group: "inbox" }] as unknown as ScopePreset[];

    expect(() =>
      render(
        <IntegrationScopeBar
          testId="scope"
          savedMenuTestId="saved"
          kinds={[{ value: "pr", label: "Pull requests" }]}
          selected={{ kind: "pr", source: "preset", id: "open" }}
          onSelect={vi.fn()}
          presetsByKind={() => presets}
          savedPresets={[]}
          onDeleteSaved={vi.fn()}
          canSaveCurrent={false}
          onSaveCurrent={vi.fn()}
        />,
      ),
    ).not.toThrow();
    expect(screen.getByRole("button", { name: "Open" })).toBeTruthy();
  });
});
