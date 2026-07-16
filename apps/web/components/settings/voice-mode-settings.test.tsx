import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "./settings-save-provider";
import { VoiceModeSettings } from "./voice-mode-settings";

const updateUserSettingsMock = vi.fn();
const setUserSettingsMock = vi.fn();
const MIC_BUTTON_LABEL = "Show the mic button on the chat composer";
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

    fireEvent.click(screen.getByRole("switch", { name: MIC_BUTTON_LABEL }));

    expect(updateUserSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    expect(
      screen.getByRole("switch", { name: MIC_BUTTON_LABEL }).getAttribute("data-settings-dirty"),
    ).toBe("true");
    expect(screen.getByTestId("voice-enable-card").getAttribute("data-settings-dirty")).toBe(
      "true",
    );

    await saveContributor?.save(saveContributor.revision);

    expect(updateUserSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({ voice_mode: expect.objectContaining({ enabled: false }) }),
    );
  });

  it("rebases unchanged shortcuts before saving a voice draft", async () => {
    updateUserSettingsMock.mockResolvedValue(undefined);
    render(<VoiceModeSettings />);

    fireEvent.click(screen.getByRole("switch", { name: MIC_BUTTON_LABEL }));
    state.userSettings = {
      ...state.userSettings,
      keyboardShortcuts: { voice_toggle: { key: "v" } },
    };
    if (!saveContributor) throw new Error("Save contributor was not registered");

    await act(async () => saveContributor?.save(saveContributor.revision));

    expect(updateUserSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({ keyboard_shortcuts: { voice_toggle: { key: "v" } } }),
    );
    expect(saveContributor?.isDirty).toBe(false);
  });

  it("preserves a voice edit made while a save is in flight", async () => {
    let resolveSave: () => void = () => undefined;
    updateUserSettingsMock.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveSave = resolve;
      }),
    );
    render(<VoiceModeSettings />);
    const toggle = screen.getByRole("switch", {
      name: MIC_BUTTON_LABEL,
    });
    fireEvent.click(toggle);
    if (!saveContributor) throw new Error("Save contributor was not registered");
    let savePromise!: Promise<void>;

    act(() => {
      savePromise = Promise.resolve(saveContributor?.save(saveContributor.revision));
    });
    fireEvent.click(toggle);
    await act(async () => {
      resolveSave();
      await savePromise;
    });

    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(saveContributor?.isDirty).toBe(true);
  });
});
