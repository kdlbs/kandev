import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getSoundPreferences, setSoundPreferences } from "@/lib/notifications/sound";
import { NotificationSoundSection } from "./notification-sound-section";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SettingsPageTemplate } from "./settings-page-template";

const DIRTY = "true";
const DIRTY_ATTRIBUTE = "data-settings-dirty";
const SOUND_TOGGLE_NAME = "Enable notification sound";

function NotificationPageHarness({ onProviderSave }: { onProviderSave: () => void }) {
  const [soundIsDirty, setSoundIsDirty] = useState(false);
  return (
    <SettingsPageTemplate
      title="Notifications"
      isDirty={false}
      cardIsDirty={soundIsDirty}
      saveStatus="idle"
      onSave={onProviderSave}
    >
      <NotificationSoundSection onDirtyChange={setSoundIsDirty} />
    </SettingsPageTemplate>
  );
}

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

    const toggle = screen.getByRole("switch", { name: SOUND_TOGGLE_NAME });
    fireEvent.click(toggle);

    expect(getSoundPreferences()).toEqual({ enabled: false, presetId: "plim" });
    expect(toggle.getAttribute(DIRTY_ATTRIBUTE)).toBe(DIRTY);
    expect(screen.getByTestId("notification-sound-group").getAttribute(DIRTY_ATTRIBUTE)).toBe(
      DIRTY,
    );

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "plim" }));
    await waitFor(() => expect(toggle.getAttribute(DIRTY_ATTRIBUTE)).toBe("false"));
  });

  it("reports dirtiness so the notifications parent card can highlight", async () => {
    const onDirtyChange = vi.fn();
    render(
      <SettingsSaveProvider>
        <NotificationSoundSection onDirtyChange={onDirtyChange} />
      </SettingsSaveProvider>,
    );

    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false));
    fireEvent.click(screen.getByRole("switch", { name: SOUND_TOGGLE_NAME }));
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true));
  });

  it("does not register the provider save for a sound-only parent-card highlight", async () => {
    const onProviderSave = vi.fn();
    render(
      <SettingsSaveProvider>
        <NotificationPageHarness onProviderSave={onProviderSave} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("switch", { name: SOUND_TOGGLE_NAME }));
    const parentCard = screen.getByText("Notification Sound").closest('[data-slot="card"]');
    await waitFor(() => expect(parentCard?.getAttribute(DIRTY_ATTRIBUTE)).toBe(DIRTY));

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(getSoundPreferences().enabled).toBe(true));
    expect(onProviderSave).not.toHaveBeenCalled();
  });
});
