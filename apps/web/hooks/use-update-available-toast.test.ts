import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";
import type { useUpdateAvailableToast as UseUpdateAvailableToastHook } from "./use-update-available-toast";

let mockNotification: UpdateAvailableNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();
const mockNativeIsAvailable = vi.fn(() => false);
const mockNativeShow = vi.fn().mockResolvedValue("shown");
const UPDATE_VERSION = "v1.2.3";
const UPDATE_TITLE = "Kandev update available";

function updateNotification(): UpdateAvailableNotification {
  return {
    version: UPDATE_VERSION,
    title: UPDATE_TITLE,
    body: `Kandev ${UPDATE_VERSION} is available.`,
    occurrence_id: UPDATE_VERSION,
  };
}

function runtimeNotification(): UpdateAvailableNotification {
  return {
    ...updateNotification(),
    agent_name: "gemini",
    runtime_id: "npm:@google/gemini-cli",
    display_name: "Gemini",
    previous_version: "1.0.0",
    version: "2.0.0",
    occurrence_id: "runtime-gemini-2",
    runtime_update_status: "available",
  };
}

function summaryNotification(): UpdateAvailableNotification {
  return {
    occurrence_id: "summary-codex-gemini",
    notification_kind: "agent_runtime_summary",
    runtime_updates: [
      {
        occurrence_id: "codex-3",
        agent_name: "codex-app-server",
        runtime_id: "npm:@openai/codex",
        display_name: "Codex",
        previous_version: "1.0.0",
        version: "3.0.0",
      },
      {
        occurrence_id: "gemini-2",
        agent_name: "gemini",
        runtime_id: "npm:@google/gemini-cli",
        display_name: "Gemini",
        previous_version: "1.0.0",
        version: "2.0.0",
      },
    ],
    url: "/settings/agents#runtime-updates",
    title: "2 agent runtime updates available",
    body: "Review runtime versions in Settings > Agents.",
    version: "",
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      updateAvailableNotification: mockNotification,
      setUpdateAvailableNotification: mockClearNotification,
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: () => mockNativeIsAvailable(),
    show: (request: unknown) => mockNativeShow(request),
  },
}));

let useUpdateAvailableToast: typeof UseUpdateAvailableToastHook;

async function resetUpdateToastHook() {
  vi.clearAllMocks();
  vi.resetModules();
  mockNativeIsAvailable.mockReturnValue(false);
  mockNotification = null;
  ({ useUpdateAvailableToast } = await import("./use-update-available-toast"));
}

describe("useUpdateAvailableToast delivery", () => {
  beforeEach(resetUpdateToastHook);

  it("does nothing when there is no notification", () => {
    renderHook(() => useUpdateAvailableToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("always shows an in-app toast for a Local update occurrence", () => {
    mockNotification = updateNotification();
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: UPDATE_TITLE,
        description: expect.stringContaining(UPDATE_VERSION),
        placement: "top",
      }),
    );
    expect(mockNativeShow).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("also attempts native delivery without replacing the toast", () => {
    mockNotification = updateNotification();
    mockNativeIsAvailable.mockReturnValue(true);
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockNativeShow).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId: `system.update_available:${UPDATE_VERSION}`,
        title: UPDATE_TITLE,
      }),
    );
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-003.3, AC-AGENTS-RUNTIME-NOTIFY-003.8
  it("shows one localized summary with direct Settings navigation on browser and native paths", () => {
    mockNotification = summaryNotification();
    mockNativeIsAvailable.mockReturnValue(true);

    renderHook(() => useUpdateAvailableToast());

    const toast = mockToast.mock.calls[0]?.[0];
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(toast).toMatchObject({
      title: "2 agent runtime updates available",
      action: { href: "/settings/agents#runtime-updates", label: "Review updates" },
    });
    expect(mockNativeShow).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId: "system.update_available:summary-codex-gemini",
        title: toast.title,
        body: toast.description,
      }),
    );
  });

  it("retains the toast when native delivery is denied", () => {
    mockNotification = updateNotification();
    mockNativeIsAvailable.mockReturnValue(true);
    mockNativeShow.mockResolvedValueOnce("permission-denied");
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(mockNativeShow).toHaveBeenCalledTimes(1);
  });
});

describe("useUpdateAvailableToast occurrence behavior", () => {
  beforeEach(resetUpdateToastHook);

  it("deduplicates repeated notifications for the same version", () => {
    mockNotification = updateNotification();
    const { rerender } = renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();
    mockNotification = updateNotification();
    rerender();
    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.3
  it("names each runtime, links to its controls, and does not collapse equal versions across agents", async () => {
    ({ useUpdateAvailableToast } = await import("./use-update-available-toast"));
    mockNotification = runtimeNotification();
    const { rerender } = renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenLastCalledWith(
      expect.objectContaining({
        title: expect.stringContaining("Gemini"),
        action: { href: "/settings/agents#runtime-update-gemini", label: expect.any(String) },
      }),
    );
    mockNotification = {
      ...mockNotification,
      agent_name: "codex-app-server",
      display_name: "Codex",
      occurrence_id: "runtime-codex-2",
    };
    rerender();
    expect(mockToast).toHaveBeenCalledTimes(2);
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-002.6
  it("does not claim an interrupted activation failed or succeeded", () => {
    mockNotification = {
      ...runtimeNotification(),
      occurrence_id: "interrupted-attempt",
      runtime_update_status: "interrupted",
    };
    renderHook(() => useUpdateAvailableToast());
    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ description: expect.stringContaining("could not be confirmed") }),
    );
  });
});
