import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "../settings-save-provider";
import type { MessageQueueSettingsResponse } from "@/lib/types/system";

const fetchSettingsMock = vi.fn();
const updateSettingsMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
let currentRole: "admin" | "member" | undefined;
const MAXIMUM_LABEL = "Maximum messages per session";
const MERGE_TOGGLE_LABEL = "Enable queued message merging";
const AUTO_MERGE_TOGGLE_LABEL = "Automatically merge consecutive messages";
const EFFECTIVE_VALUE_ID = "message-queue-effective-value";
const ARIA_PRESSED = "aria-pressed";
const SAVE_FAILED = "save failed";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchMessageQueueSettings: (...args: unknown[]) => fetchSettingsMock(...args),
  updateMessageQueueSettings: (...args: unknown[]) => updateSettingsMock(...args),
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

import { MessageQueueSettings } from "./message-queue-settings";

type ResponseOverrides = {
  configured?: number;
  effective?: number;
  source?: MessageQueueSettingsResponse["effective"]["source"];
  locked?: boolean;
  mergeEnabled?: boolean;
  autoMergeEnabled?: boolean;
};

function response(overrides: ResponseOverrides = {}): MessageQueueSettingsResponse {
  const configured = overrides.configured ?? 10;
  const effective = overrides.effective ?? configured;
  const source = overrides.source ?? "default";
  const locked = overrides.locked ?? false;
  const mergeEnabled = overrides.mergeEnabled ?? true;
  const autoMergeEnabled = overrides.autoMergeEnabled ?? true;
  return {
    settings: {
      max_per_session: configured,
      merge_enabled: mergeEnabled,
      auto_merge_enabled: autoMergeEnabled,
    },
    effective: {
      max_per_session: effective,
      source,
      locked,
      merge_enabled: mergeEnabled,
      auto_merge_enabled: autoMergeEnabled,
    },
  };
}

/** Every save/discard test needs the contributor the component just
 * registered; centralizing the "not registered yet" guard keeps that
 * assertion's message out of `sonarjs/no-duplicate-string` territory. */
function requireContributor(): SettingsSaveContributor {
  if (!saveContributor) throw new Error("save contributor was not registered");
  return saveContributor;
}

function mergeToggle(): HTMLElement {
  return screen.getByRole("button", { name: MERGE_TOGGLE_LABEL });
}

function autoMergeToggle(): HTMLElement {
  return screen.getByRole("button", { name: AUTO_MERGE_TOGGLE_LABEL });
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

describe("MessageQueueSettings — per-session limit", () => {
  it("shows loading, configured, effective, and default-source states", async () => {
    let resolveLoad: (value: MessageQueueSettingsResponse) => void = () => {};
    fetchSettingsMock.mockReturnValueOnce(
      new Promise<MessageQueueSettingsResponse>((resolve) => {
        resolveLoad = resolve;
      }),
    );

    render(<MessageQueueSettings />);
    expect(screen.getByText("Loading message queue settings…")).toBeTruthy();

    resolveLoad(response());

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(input.getAttribute("value")).toBe("10");
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("10");
    expect(screen.getByTestId("message-queue-source").textContent).toBe("Default");
  });

  it("explains unlimited mode and keeps a mobile-safe single-column surface", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({ configured: 0, effective: 0, source: "setting" }),
    );
    render(<MessageQueueSettings />);

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("Unlimited");
    expect(screen.getByText(/Set 0 for unlimited/)).toBeTruthy();
    expect(input.className).toContain("h-11");
    const root = screen.getByTestId("message-queue-settings");
    expect(root.className).toContain("min-w-0");
    expect(root.querySelectorAll(".overflow-y-auto")).toHaveLength(0);
  });

  it("stages an admin edit until the shared contributor saves it", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response({ configured: 25, effective: 25, source: "setting" }),
    );
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);

    fireEvent.change(input, { target: { value: "25" } });

    expect(updateSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ max_per_session: 25 });
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("25");
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
  });

  it.each(["-1", "2.5", ""])("rejects invalid draft %j", async (draft) => {
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);

    fireEvent.change(input, { target: { value: draft } });

    expect(saveContributor?.isDirty).toBe(true);
    expect(saveContributor?.canSave).toBe(false);
    expect(saveContributor?.invalidReason).toBe("Enter a whole number of 0 or greater.");
  });

  it("shows the effective environment value and locks editing", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({ configured: 25, effective: 50, source: "environment", locked: true }),
    );
    render(<MessageQueueSettings />);

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(input).toHaveProperty("disabled", true);
    expect(input.getAttribute("value")).toBe("25");
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("50");
    expect(screen.getByTestId("message-queue-source").textContent).toBe("Environment");
    expect(screen.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/)).toBeTruthy();
  });

  it("shows a configuration value as a locked configuration source", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({
        configured: 25,
        effective: 41,
        source: "configuration" as MessageQueueSettingsResponse["effective"]["source"],
        locked: true,
      }),
    );
    render(<MessageQueueSettings />);

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(input).toHaveProperty("disabled", true);
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("41");
    expect(screen.getByTestId("message-queue-source").textContent).toBe("Configuration");
    expect(screen.getByText(/Managed by configuration/)).toBeTruthy();
    expect(screen.queryByText(/KANDEV_QUEUE_MAX_PER_SESSION/)).toBeNull();
  });
});

describe("MessageQueueSettings — permissions and recovery", () => {
  it("keeps members read-only while showing the setting", async () => {
    currentRole = "member";
    render(<MessageQueueSettings />);

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(input).toHaveProperty("disabled", true);
    expect(screen.getByText("Only administrators can change this setting.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(false);
  });

  it("shows a retryable load error", async () => {
    fetchSettingsMock.mockRejectedValueOnce(new Error("offline"));
    render(<MessageQueueSettings />);

    expect(await screen.findByText("Message queue settings could not be loaded.")).toBeTruthy();
    fetchSettingsMock.mockResolvedValueOnce(response());
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByLabelText(MAXIMUM_LABEL)).toBeTruthy();
  });

  it("reports save failure and keeps the draft dirty", async () => {
    updateSettingsMock.mockRejectedValueOnce(new Error(SAVE_FAILED));
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "20" } });
    const contributor = requireContributor();

    await act(async () => {
      await expect(contributor.save(contributor.revision)).rejects.toThrow(SAVE_FAILED);
    });

    expect(screen.getByText("Failed to save message queue settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(true);
  });

  it("discards a draft back to the authoritative configured value", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({ configured: 12, effective: 12, source: "setting" }),
    );
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "30" } });
    const contributor = requireContributor();

    act(() => contributor.discard());

    expect(input.getAttribute("value")).toBe("12");
    expect(saveContributor?.isDirty).toBe(false);
  });
});

describe("MessageQueueSettings — card surface", () => {
  it("fills the available page width like the other settings boxes", async () => {
    fetchSettingsMock.mockResolvedValueOnce(response());
    render(<MessageQueueSettings />);

    const root = await screen.findByTestId("message-queue-settings");
    expect(root.className).toContain("w-full");
    expect(root.className).not.toMatch(/\bmax-w-/);
  });
});

describe("MessageQueueSettings — merge toggle", () => {
  it("is enabled by default and shows its limitations notice", async () => {
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(mergeToggle().getAttribute(ARIA_PRESSED)).toBe("true");
    expect(
      screen.getByText(/Lets you fold a queued message into the message directly above it/),
    ).toBeTruthy();
    expect(
      screen.getByText(/Only adjacent messages from the same sender can be merged/),
    ).toBeTruthy();
    expect(
      screen.getByText(/combined, deduplicated entity references would exceed 100/),
    ).toBeTruthy();
  });

  it("stages a toggle change and PATCHes merge_enabled without touching max_per_session", async () => {
    updateSettingsMock.mockResolvedValueOnce(response({ mergeEnabled: false }));
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    fireEvent.click(mergeToggle());

    expect(updateSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ merge_enabled: false });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
    expect(mergeToggle().getAttribute(ARIA_PRESSED)).toBe("false");
  });

  it("sends both fields in one PATCH when max_per_session and merge_enabled are both dirty", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response({ configured: 25, effective: 25, source: "setting", mergeEnabled: false }),
    );
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);

    fireEvent.change(input, { target: { value: "25" } });
    fireEvent.click(mergeToggle());
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({
      max_per_session: 25,
      merge_enabled: false,
    });
  });

  it("stays editable for admins even when max_per_session is environment-locked", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({ configured: 25, effective: 50, source: "environment", locked: true }),
    );
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(mergeToggle()).toHaveProperty("disabled", false);
  });

  it("is disabled for members", async () => {
    currentRole = "member";
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(mergeToggle()).toHaveProperty("disabled", true);
  });

  it("discards a toggle change back to the authoritative value", async () => {
    fetchSettingsMock.mockResolvedValueOnce(response());
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(mergeToggle());
    const contributor = requireContributor();

    act(() => contributor.discard());

    expect(mergeToggle().getAttribute(ARIA_PRESSED)).toBe("true");
    expect(saveContributor?.isDirty).toBe(false);
  });
});

describe("MessageQueueSettings — automatic merge toggle", () => {
  it("is on by default and explains separate-message fallback", async () => {
    render(<MessageQueueSettings />);

    const toggle = await screen.findByRole("button", { name: AUTO_MERGE_TOGGLE_LABEL });
    expect(toggle.getAttribute(ARIA_PRESSED)).toBe("true");
    expect(screen.getByText(/stays as a separate queued message/)).toBeTruthy();
    const touchTarget = screen.getByTestId("message-queue-auto-merge-touch-target");
    expect(touchTarget.className).toContain("min-h-11");
    expect(touchTarget.className).toContain("min-w-11");
  });

  it("saves only auto_merge_enabled when it is the only changed draft", async () => {
    updateSettingsMock.mockResolvedValueOnce(response({ autoMergeEnabled: false }));
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(autoMergeToggle());
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ auto_merge_enabled: false });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
  });

  it("saves capacity, manual merge, and automatic merge in one PATCH", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response({
        configured: 25,
        effective: 25,
        source: "setting",
        mergeEnabled: false,
        autoMergeEnabled: false,
      }),
    );
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "25" } });
    fireEvent.click(mergeToggle());
    fireEvent.click(autoMergeToggle());
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({
      max_per_session: 25,
      merge_enabled: false,
      auto_merge_enabled: false,
    });
  });

  it("stays editable for admins under a capacity environment lock", async () => {
    fetchSettingsMock.mockResolvedValueOnce(
      response({ configured: 25, effective: 50, source: "environment", locked: true }),
    );
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(autoMergeToggle()).toHaveProperty("disabled", false);
  });

  it("is read-only for members", async () => {
    currentRole = "member";
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(autoMergeToggle()).toHaveProperty("disabled", true);
  });

  it("discards back to the authoritative automatic-merge value", async () => {
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(autoMergeToggle());
    const contributor = requireContributor();

    act(() => contributor.discard());

    expect(autoMergeToggle().getAttribute(ARIA_PRESSED)).toBe("true");
    expect(saveContributor?.isDirty).toBe(false);
  });

  it("does not overwrite a newer local toggle change when a save resolves", async () => {
    let resolveSave: (value: MessageQueueSettingsResponse) => void = () => {};
    updateSettingsMock.mockReturnValueOnce(
      new Promise<MessageQueueSettingsResponse>((resolve) => {
        resolveSave = resolve;
      }),
    );
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(autoMergeToggle());
    const submittedContributor = requireContributor();
    let savePromise: Promise<void> = Promise.resolve();

    act(() => {
      savePromise = Promise.resolve(submittedContributor.save(submittedContributor.revision));
    });
    fireEvent.click(autoMergeToggle());
    resolveSave(response({ source: "setting", autoMergeEnabled: false }));
    await act(async () => savePromise);

    expect(autoMergeToggle().getAttribute(ARIA_PRESSED)).toBe("true");
    expect(saveContributor?.isDirty).toBe(true);
  });

  it("keeps a failed automatic-merge draft dirty and reports the error", async () => {
    updateSettingsMock.mockRejectedValueOnce(new Error(SAVE_FAILED));
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(autoMergeToggle());
    const contributor = requireContributor();

    await act(async () => {
      await expect(contributor.save(contributor.revision)).rejects.toThrow(SAVE_FAILED);
    });

    expect(screen.getByText("Failed to save message queue settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(true);
  });
});
