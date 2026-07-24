import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { UpdateNotificationsCard } from "./update-notifications-card";

const mockSave = vi.fn();
const TOGGLE_NAME = "Enable update notifications";
const DIRTY_ATTRIBUTE = "data-settings-dirty";

vi.mock("@/lib/api/domains/system-api", () => ({
  saveUpdateNotificationSettings: (...args: unknown[]) => mockSave(...args),
}));

function renderCard(initial?: { enabled: boolean; channel: string }) {
  const initialState: Record<string, unknown> | undefined = initial
    ? { system: { updateNotificationSettings: initial } }
    : undefined;
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <SettingsSaveProvider>
          <UpdateNotificationsCard />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockSave.mockImplementation((settings) => Promise.resolve(settings));
});

afterEach(() => {
  cleanup();
});

describe("UpdateNotificationsCard", () => {
  it("renders the saved enable toggle and channel from hydrated state", async () => {
    renderCard({ enabled: true, channel: "desktop" });

    const toggle = await screen.findByRole("switch", { name: TOGGLE_NAME });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText("Desktop notification")).toBeTruthy();
  });

  it("falls back to the backend default without fetching when no hydrated state is available", async () => {
    renderCard();

    const toggle = await screen.findByRole("switch", { name: TOGGLE_NAME });
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText("Both")).toBeTruthy();
  });

  it("hides the channel selector once disabled and persists through the floating Save action", async () => {
    renderCard({ enabled: true, channel: "both" });

    const toggle = await screen.findByRole("switch", { name: TOGGLE_NAME });
    fireEvent.click(toggle);

    expect(screen.queryByLabelText("Update notification channel")).toBeNull();
    expect(toggle.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(mockSave).toHaveBeenCalledWith({ enabled: false, channel: "both" }));
    await waitFor(() => expect(toggle.getAttribute(DIRTY_ATTRIBUTE)).toBe("false"));
  });

  it("marks the trigger dirty on channel change and persists the new channel on save", async () => {
    renderCard({ enabled: true, channel: "in_view" });

    await screen.findByText("In-app banner");
    const trigger = screen.getByRole("combobox", { name: "Update notification channel" });
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("option", { name: "Both" }));

    expect(screen.getByText("Both")).toBeTruthy();
    expect(trigger.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(mockSave).toHaveBeenCalledWith({ enabled: true, channel: "both" }));
    await waitFor(() => expect(trigger.getAttribute(DIRTY_ATTRIBUTE)).toBe("false"));
  });
});
