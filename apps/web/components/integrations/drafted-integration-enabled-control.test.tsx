import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { DraftedIntegrationEnabledControl } from "./drafted-integration-enabled-control";

afterEach(() => cleanup());

describe("DraftedIntegrationEnabledControl", () => {
  it("reflects the enabled prop via aria-checked and accessible name", () => {
    render(
      <SettingsSaveProvider>
        <DraftedIntegrationEnabledControl
          id="github"
          name="GitHub"
          enabled={true}
          persist={vi.fn()}
        />
      </SettingsSaveProvider>,
    );

    const control = screen.getByRole("switch", { name: "Enable GitHub" });
    expect(control.getAttribute("aria-checked")).toBe("true");
    expect(control.getAttribute("data-settings-dirty")).toBe("false");
  });

  it("starts unchecked when enabled is false", () => {
    render(
      <SettingsSaveProvider>
        <DraftedIntegrationEnabledControl
          id="github"
          name="GitHub"
          enabled={false}
          persist={vi.fn()}
        />
      </SettingsSaveProvider>,
    );

    expect(screen.getByRole("switch", { name: "Enable GitHub" }).getAttribute("aria-checked")).toBe(
      "false",
    );
  });

  it("marks the switch dirty on toggle and persists only after save", async () => {
    const persist = vi.fn();
    render(
      <SettingsSaveProvider>
        <DraftedIntegrationEnabledControl
          id="github"
          name="GitHub"
          enabled={true}
          persist={persist}
        />
      </SettingsSaveProvider>,
    );

    const control = screen.getByRole("switch", { name: "Enable GitHub" });
    fireEvent.click(control);

    expect(control.getAttribute("aria-checked")).toBe("false");
    expect(control.getAttribute("data-settings-dirty")).toBe("true");
    expect(persist).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(persist).toHaveBeenCalledWith(false));
    await waitFor(() => expect(control.getAttribute("data-settings-dirty")).toBe("false"));
  });
});
