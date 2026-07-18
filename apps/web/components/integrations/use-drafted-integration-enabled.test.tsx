import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { useDraftedIntegrationEnabled } from "./use-drafted-integration-enabled";

function Harness({ persist }: { persist: (enabled: boolean) => void }) {
  const [enabled, setEnabled] = useState(true);
  const draft = useDraftedIntegrationEnabled({
    id: "test-enabled",
    enabled,
    persist: (next) => {
      persist(next);
      setEnabled(next);
    },
  });
  return (
    <button data-settings-dirty={draft.isDirty} onClick={() => draft.setEnabled(!draft.enabled)}>
      Toggle
    </button>
  );
}

describe("useDraftedIntegrationEnabled", () => {
  it("persists only after the shared save action is pressed", async () => {
    const persist = vi.fn();
    render(
      <SettingsSaveProvider>
        <Harness persist={persist} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle" }));
    expect(persist).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Toggle" }).getAttribute("data-settings-dirty")).toBe(
      "true",
    );

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(persist).toHaveBeenCalledWith(false));
  });
});
