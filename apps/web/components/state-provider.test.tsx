import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { StateProvider, useAppStore } from "./state-provider";

function ShowMetricsPreference({ label }: { label: string }) {
  const enabled = useAppStore((state) => state.userSettings.systemMetricsDisplay.showInTopbar);
  return (
    <div>
      {label}:{enabled ? "on" : "off"}
    </div>
  );
}

function EnableMetricsFromNestedProvider() {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);
  return (
    <button
      type="button"
      onClick={() =>
        setUserSettings({
          ...userSettings,
          systemMetricsDisplay: { showInTopbar: true },
          loaded: true,
        })
      }
    >
      Enable metrics
    </button>
  );
}

describe("StateProvider", () => {
  it("reuses the parent store for nested route providers", async () => {
    render(
      <StateProvider>
        <ShowMetricsPreference label="root" />
        <StateProvider
          initialState={{
            userSettings: {
              ...defaultSettingsState.userSettings,
              systemMetricsDisplay: { showInTopbar: false },
              loaded: true,
            },
          }}
        >
          <EnableMetricsFromNestedProvider />
        </StateProvider>
      </StateProvider>,
    );

    expect(screen.getByText("root:off")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Enable metrics" }));
    expect(await screen.findByText("root:on")).toBeTruthy();
  });
});
