import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "../settings-save-provider";
import type { SessionCapacitySettingsResponse } from "@/lib/types/system";

const ENABLE_LABEL = "Enable the instance session limit";
const MAXIMUM_LABEL = "Maximum automatic sessions";
const NO_LIMIT = "No session limit";
const ADMIN_ONLY = "Only administrators can change this setting.";
const VALIDATION = "Enter a positive whole number.";
const fetchSettingsMock = vi.fn();
const updateSettingsMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
let currentRole: "admin" | "member" | undefined;

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchSessionCapacitySettings: (...args: unknown[]) => fetchSettingsMock(...args),
  updateSessionCapacitySettings: (...args: unknown[]) => updateSettingsMock(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { auth: { user?: { role: string } } }) => unknown) =>
    selector({ auth: { user: currentRole ? { role: currentRole } : undefined } }),
}));

vi.mock("../settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    saveContributor = contributor;
  },
}));

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({
    checked,
    disabled,
    "aria-label": ariaLabel,
    onCheckedChange,
  }: {
    checked: boolean;
    disabled: boolean;
    "aria-label": string;
    onCheckedChange: (checked: boolean) => void;
  }) => (
    <button
      aria-label={ariaLabel}
      aria-pressed={checked}
      disabled={disabled}
      type="button"
      onClick={() => onCheckedChange(!checked)}
    />
  ),
}));

import { SessionCapacitySettings } from "./session-capacity-settings";

function response(
  overrides: Partial<{
    enabled: boolean;
    maximum: number;
    effectiveEnabled: boolean;
    effectiveMaximum: number;
    source: SessionCapacitySettingsResponse["effective"]["source"];
    locked: boolean;
  }> = {},
): SessionCapacitySettingsResponse {
  const enabled = overrides.enabled ?? false;
  const maximum = overrides.maximum ?? 5;
  const effectiveEnabled = overrides.effectiveEnabled ?? enabled;
  const effectiveMaximum = overrides.effectiveMaximum ?? (effectiveEnabled ? maximum : 0);
  return {
    settings: { enabled, max_sessions: maximum },
    effective: {
      enabled: effectiveEnabled,
      max_sessions: effectiveMaximum,
      source: overrides.source ?? (enabled ? "setting" : "default"),
      locked: overrides.locked ?? false,
    },
  };
}

function requireContributor(): SettingsSaveContributor {
  if (!saveContributor) throw new Error("save contributor was not registered");
  return saveContributor;
}

beforeEach(() => {
  fetchSettingsMock.mockReset();
  updateSettingsMock.mockReset();
  fetchSettingsMock.mockResolvedValue(response());
  currentRole = "admin";
  saveContributor = null;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SessionCapacitySettings drafts", () => {
  it("loads disabled by default and does not show an active limit", async () => {
    render(<SessionCapacitySettings />);

    expect(await screen.findByText(NO_LIMIT)).toBeTruthy();
    expect(screen.queryByLabelText(MAXIMUM_LABEL)).toBeNull();
    expect(screen.getByTestId("session-capacity-source").textContent).toBe("Default");
  });

  it("stages an atomic enable and maximum edit until the contributor saves", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response({ enabled: true, maximum: 8, effectiveEnabled: true, effectiveMaximum: 8 }),
    );
    render(<SessionCapacitySettings />);

    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "8" } });

    expect(updateSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    const contributor = requireContributor();
    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ enabled: true, max_sessions: 8 });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
    expect(screen.getByTestId("session-capacity-effective-value").textContent).toBe("8");
  });

  it("rejects an invalid enabled maximum", async () => {
    render(<SessionCapacitySettings />);
    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, {
      target: { value: "0" },
    });

    expect(saveContributor?.canSave).toBe(false);
    expect(saveContributor?.invalidReason).toBe(VALIDATION);
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input.getAttribute("aria-describedby")?.split(/\s+/).sort()).toEqual(
      ["session-capacity-maximum-error", "session-capacity-maximum-help"].sort(),
    );
    expect(screen.getByTestId("session-capacity-maximum-error").textContent).toBe(VALIDATION);
  });

  it("allows turning the draft off after an invalid edit and retains the saved maximum", async () => {
    updateSettingsMock.mockResolvedValueOnce(response({ enabled: false, maximum: 5 }));
    render(<SessionCapacitySettings />);
    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    fireEvent.change(await screen.findByLabelText(MAXIMUM_LABEL), {
      target: { value: "bad" },
    });
    fireEvent.click(toggle);

    const contributor = requireContributor();
    expect(contributor.canSave).toBe(true);
    await act(async () => contributor.save(contributor.revision));
    expect(updateSettingsMock).toHaveBeenCalledWith({ enabled: false, max_sessions: 5 });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
  });

  it("normalizes a valid maximum draft after saving", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response({ enabled: true, maximum: 5, effectiveEnabled: true, effectiveMaximum: 5 }),
    );
    render(<SessionCapacitySettings />);
    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "05" } });

    const contributor = requireContributor();
    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ enabled: true, max_sessions: 5 });
    await waitFor(() => {
      expect(input.getAttribute("value")).toBe("5");
      expect(saveContributor?.isDirty).toBe(false);
    });
  });
});

describe("SessionCapacitySettings access and recovery", () => {
  it("shows effective environment state and prevents edits", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({
        enabled: false,
        maximum: 5,
        effectiveEnabled: true,
        effectiveMaximum: 9,
        source: "environment",
        locked: true,
      }),
    );
    render(<SessionCapacitySettings />);

    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    expect(toggle).toHaveProperty("disabled", true);
    expect(screen.getByLabelText(MAXIMUM_LABEL)).toHaveProperty("disabled", true);
    expect(screen.getByTestId("session-capacity-effective-value").textContent).toBe("9");
    expect(screen.getByText(/KANDEV_MAX_CONCURRENT_SESSIONS/)).toBeTruthy();
  });

  it("keeps members read-only", async () => {
    currentRole = "member";
    fetchSettingsMock.mockResolvedValueOnce(
      response({
        enabled: true,
        maximum: 7,
        effectiveEnabled: true,
        effectiveMaximum: 7,
        source: "setting",
      }),
    );
    render(<SessionCapacitySettings />);

    expect(await screen.findByLabelText(MAXIMUM_LABEL)).toHaveProperty("disabled", true);
    expect(screen.getByText(ADMIN_ONLY)).toBeTruthy();
  });

  it("preserves a newer edit when an earlier save resolves", async () => {
    let resolveSave: (value: SessionCapacitySettingsResponse) => void = () => {};
    updateSettingsMock.mockReturnValueOnce(
      new Promise<SessionCapacitySettingsResponse>((resolve) => {
        resolveSave = resolve;
      }),
    );
    render(<SessionCapacitySettings />);
    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "8" } });
    const submittedContributor = requireContributor();
    let savePromise: Promise<void> = Promise.resolve();
    act(() => {
      savePromise = Promise.resolve(submittedContributor.save(submittedContributor.revision));
    });
    fireEvent.change(input, { target: { value: "9" } });
    resolveSave(
      response({ enabled: true, maximum: 8, effectiveEnabled: true, effectiveMaximum: 8 }),
    );
    await act(async () => savePromise);

    expect(input.getAttribute("value")).toBe("9");
    expect(saveContributor?.isDirty).toBe(true);
  });

  it("reports load and save failures without clearing the draft", async () => {
    fetchSettingsMock.mockRejectedValueOnce(new Error("offline"));
    render(<SessionCapacitySettings />);
    expect(await screen.findByText("Session capacity settings could not be loaded.")).toBeTruthy();

    fetchSettingsMock.mockResolvedValueOnce(response());
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    const toggle = await screen.findByRole("button", { name: ENABLE_LABEL });
    fireEvent.click(toggle);
    updateSettingsMock.mockRejectedValueOnce(new Error("offline"));
    const contributor = requireContributor();
    await act(async () => {
      await expect(contributor.save(contributor.revision)).rejects.toThrow("offline");
    });

    expect(screen.getByText("Failed to save session capacity settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(true);
  });
});
