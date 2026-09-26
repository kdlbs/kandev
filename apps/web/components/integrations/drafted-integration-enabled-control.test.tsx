import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "@/lib/plugins/host-api";
import { DraftedIntegrationEnabledControl } from "./drafted-integration-enabled-control";

afterEach(cleanup);

describe("DraftedIntegrationEnabledControl", () => {
  it("exposes the current enabled state and an accessible integration name", () => {
    const persist = vi.fn();

    render(
      <SettingsSaveProvider>
        <DraftedIntegrationEnabledControl id="github" name="GitHub" enabled persist={persist} />
      </SettingsSaveProvider>,
    );

    const toggle = screen.getByRole("switch", { name: "Enable GitHub" });
    expect(toggle.getAttribute("id")).toBe("github-enabled");
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("false");
  });

  it("starts unchecked when the integration is disabled", () => {
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

  it("keeps changes local until Save changes persists the draft", async () => {
    const persist = vi.fn().mockResolvedValue(undefined);

    render(
      <SettingsSaveProvider>
        <DraftedIntegrationEnabledControl id="github" name="GitHub" enabled persist={persist} />
      </SettingsSaveProvider>,
    );

    const toggle = screen.getByRole("switch", { name: "Enable GitHub" });
    fireEvent.click(toggle);

    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(persist).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(persist).toHaveBeenCalledWith(false));
    await waitFor(() => expect(toggle.getAttribute("data-settings-dirty")).toBe("false"));
  });

  it("scopes the host-provided control id to its plugin", () => {
    const host = buildHostApi("plugin-control", createAppStore());
    const Control = host.ui.IntegrationEnabledControl as ComponentType<{
      id: string;
      enabled: boolean;
      persist: (enabled: boolean) => Promise<void> | void;
      name: string;
    }>;

    render(
      <SettingsSaveProvider>
        <Control id="source-control" name="Source control" enabled persist={vi.fn()} />
      </SettingsSaveProvider>,
    );

    expect(screen.getByRole("switch", { name: "Enable Source control" }).getAttribute("id")).toBe(
      "plugin:plugin-control:source-control-enabled",
    );
  });
});
