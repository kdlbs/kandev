import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getSoundPreferences, setSoundPreferences } from "@/lib/notifications/sound";
import { NotificationSoundSection } from "./notification-sound-section";
import { SettingsSaveProvider } from "./settings-save-provider";

beforeEach(() => {
  window.localStorage.clear();
  setSoundPreferences({ enabled: false, presetId: "plim" });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("NotificationSoundSection", () => {
  it("persists a dirty sound preference only through the route Save action", async () => {
    render(
      <SettingsSaveProvider>
        <NotificationSoundSection />
      </SettingsSaveProvider>,
    );

    const toggle = screen.getByRole("switch", { name: "Enable notification sound" });
    fireEvent.click(toggle);

    expect(getSoundPreferences()).toEqual({ enabled: false, presetId: "plim" });
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByTestId("notification-sound-group").getAttribute("data-settings-dirty")).toBe(
      "true",
    );

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "plim" }));
    await waitFor(() => expect(toggle.getAttribute("data-settings-dirty")).toBe("false"));
  });
});
