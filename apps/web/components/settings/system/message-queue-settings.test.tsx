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
const EFFECTIVE_VALUE_ID = "message-queue-effective-value";

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

function response(
  configured = 10,
  effective = configured,
  source: MessageQueueSettingsResponse["effective"]["source"] = "default",
  locked = false,
  mergeEnabled = true,
): MessageQueueSettingsResponse {
  return {
    settings: { max_per_session: configured, merge_enabled: mergeEnabled },
    effective: { max_per_session: effective, source, locked, merge_enabled: mergeEnabled },
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
    fetchSettingsMock.mockResolvedValueOnce(response(0, 0, "setting"));
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
    updateSettingsMock.mockResolvedValueOnce(response(25, 25, "setting"));
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
    fetchSettingsMock.mockResolvedValueOnce(response(25, 50, "environment", true));
    render(<MessageQueueSettings />);

    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    expect(input).toHaveProperty("disabled", true);
    expect(input.getAttribute("value")).toBe("25");
    expect(screen.getByTestId(EFFECTIVE_VALUE_ID).textContent).toBe("50");
    expect(screen.getByTestId("message-queue-source").textContent).toBe("Environment");
    expect(screen.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/)).toBeTruthy();
  });

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
    updateSettingsMock.mockRejectedValueOnce(new Error("save failed"));
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "20" } });
    const contributor = requireContributor();

    await act(async () => {
      await expect(contributor.save(contributor.revision)).rejects.toThrow("save failed");
    });

    expect(screen.getByText("Failed to save message queue settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(true);
  });

  it("discards a draft back to the authoritative configured value", async () => {
    fetchSettingsMock.mockResolvedValueOnce(response(12, 12, "setting"));
    render(<MessageQueueSettings />);
    const input = await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.change(input, { target: { value: "30" } });
    const contributor = requireContributor();

    act(() => contributor.discard());

    expect(input.getAttribute("value")).toBe("12");
    expect(saveContributor?.isDirty).toBe(false);
  });
});

describe("MessageQueueSettings — merge toggle", () => {
  it("is enabled by default and shows its limitations notice", async () => {
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    expect(mergeToggle().getAttribute("aria-pressed")).toBe("true");
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
    updateSettingsMock.mockResolvedValueOnce(response(10, 10, "default", false, false));
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);

    fireEvent.click(mergeToggle());

    expect(updateSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    const contributor = requireContributor();

    await act(async () => contributor.save(contributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ merge_enabled: false });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
    expect(mergeToggle().getAttribute("aria-pressed")).toBe("false");
  });

  it("sends both fields in one PATCH when max_per_session and merge_enabled are both dirty", async () => {
    updateSettingsMock.mockResolvedValueOnce(response(25, 25, "setting", false, false));
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
    fetchSettingsMock.mockResolvedValueOnce(response(25, 50, "environment", true, true));
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
    fetchSettingsMock.mockResolvedValueOnce(response(10, 10, "default", false, true));
    render(<MessageQueueSettings />);
    await screen.findByLabelText(MAXIMUM_LABEL);
    fireEvent.click(mergeToggle());
    const contributor = requireContributor();

    act(() => contributor.discard());

    expect(mergeToggle().getAttribute("aria-pressed")).toBe("true");
    expect(saveContributor?.isDirty).toBe(false);
  });
});
