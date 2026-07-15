import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "./settings-save-provider";
import { VoiceModeSettings } from "./voice-mode-settings";

const updateUserSettingsMock = vi.fn();
const setUserSettingsMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
const initialVoiceMode = {
  enabled: true,
  engine: "auto" as const,
  language: "auto",
  mode: "toggle" as const,
  autoSend: false,
  whisperWebModel: "base" as const,
};
const state = {
  userSettings: {
    voiceMode: initialVoiceMode,
    keyboardShortcuts: {},
  },
  setUserSettings: setUserSettingsMock,
};

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettingsMock(...args),
}));

vi.mock("./settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    saveContributor = contributor;
  },
}));

afterEach(() => {
  cleanup();
  updateUserSettingsMock.mockReset();
  setUserSettingsMock.mockReset();
  state.userSettings = { voiceMode: initialVoiceMode, keyboardShortcuts: {} };
  saveContributor = null;
});

describe("VoiceModeSettings", () => {
  it("stages voice configuration until the route save runs", async () => {
    updateUserSettingsMock.mockResolvedValue(undefined);
    render(<VoiceModeSettings />);

    fireEvent.click(
      screen.getByRole("switch", { name: "Show the mic button on the chat composer" }),
    );

    expect(updateUserSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);

    await saveContributor?.save(saveContributor.revision);

    expect(updateUserSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({ voice_mode: expect.objectContaining({ enabled: false }) }),
    );
  });

  it("rebases unchanged shortcuts before saving a voice draft", async () => {
    updateUserSettingsMock.mockResolvedValue(undefined);
    render(<VoiceModeSettings />);

    fireEvent.click(
      screen.getByRole("switch", { name: "Show the mic button on the chat composer" }),
    );
    state.userSettings = {
      ...state.userSettings,
      keyboardShortcuts: { voice_toggle: { key: "v" } },
    };
    if (!saveContributor) throw new Error("Save contributor was not registered");

    await saveContributor.save(saveContributor.revision);

    expect(updateUserSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({ keyboard_shortcuts: { voice_toggle: { key: "v" } } }),
    );
  });
});
