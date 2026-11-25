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
const SOUND_SELECT_NAME = "Notification sound";
const PREVIEW_NAME = "Preview sound";

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

  it("shows the selected preset under its translated name", async () => {
    setSoundPreferences({ enabled: true, presetId: "ding" });
    render(
      <SettingsSaveProvider>
        <NotificationSoundSection />
      </SettingsSaveProvider>,
    );

    // The preset id is the persisted sentinel; only the name is translated.
    const trigger = await screen.findByRole("combobox", { name: SOUND_SELECT_NAME });
    expect(trigger.textContent).toContain("Ding");
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
    expect(screen.getAllByText("Unsaved changes").length).toBeGreaterThan(0);
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

describe("NotificationSoundSection controls", () => {
  it("keeps the preset and preview visible and disabled while sound is off", async () => {
    setSoundPreferences({ enabled: false, presetId: "ding" });
    render(
      <SettingsSaveProvider>
        <NotificationSoundSection />
      </SettingsSaveProvider>,
    );

    const selector = await screen.findByRole("combobox", { name: SOUND_SELECT_NAME });
    const preview = screen.getByRole("button", { name: PREVIEW_NAME });

    expect(selector).toBeTruthy();
    expect(preview).toBeTruthy();
    expect((selector as HTMLButtonElement).disabled).toBe(true);
    expect((preview as HTMLButtonElement).disabled).toBe(true);
    expect(selector.textContent).toContain("Ding");
    expect(screen.getByLabelText("Enable notification sound to preview it.")).toBeTruthy();

    fireEvent.click(screen.getByRole("switch", { name: SOUND_TOGGLE_NAME }));

    expect((selector as HTMLButtonElement).disabled).toBe(false);
    expect((preview as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByLabelText("Enable notification sound to preview it.")).toBeNull();
  });

  it("preserves the chosen preset across sound toggles", async () => {
    setSoundPreferences({ enabled: true, presetId: "ding" });
    render(
      <SettingsSaveProvider>
        <NotificationSoundSection />
      </SettingsSaveProvider>,
    );

    const toggle = screen.getByRole("switch", { name: SOUND_TOGGLE_NAME });
    fireEvent.click(toggle);

    const selector = await screen.findByRole("combobox", { name: SOUND_SELECT_NAME });
    expect(selector.textContent).toContain("Ding");
    expect((selector as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(toggle);
    expect(selector.textContent).toContain("Ding");
    expect((selector as HTMLButtonElement).disabled).toBe(false);
  });

  it("saves a preset draft and Reset restores the saved preset", async () => {
    setSoundPreferences({ enabled: true, presetId: "plim" });
    render(
      <SettingsSaveProvider>
        <NotificationPageHarness onProviderSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("combobox", { name: SOUND_SELECT_NAME }));
    fireEvent.click(screen.getByRole("option", { name: "Ding" }));

    // Selecting is a draft: the persisted preference changes only on route Save.
    expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "plim" });
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "ding" }));

    fireEvent.click(screen.getByRole("combobox", { name: SOUND_SELECT_NAME }));
    fireEvent.click(screen.getByRole("option", { name: "Chime" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));

    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: SOUND_SELECT_NAME }).textContent).toContain(
        "Ding",
      ),
    );
    expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "ding" });
  });
});
