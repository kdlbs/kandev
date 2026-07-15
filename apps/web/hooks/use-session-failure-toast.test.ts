import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import type { SessionFailureNotification } from "@/lib/state/slices/ui/types";

let mockNotification: SessionFailureNotification | null = null;
const mockClearNotification = vi.fn();
const mockToast = vi.fn();
let mockProviders: Array<Record<string, unknown>> = [];
let mockProvidersLoaded = true;
const WAITING_EVENT = "session.waiting_for_input";

vi.mock("@/lib/api", () => ({
  listNotificationProviders: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      sessionFailureNotification: mockNotification,
      setSessionFailureNotification: mockClearNotification,
      notificationProviders: {
        items: mockProviders,
        loaded: mockProvidersLoaded,
      },
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

import { useSessionFailureToast } from "./use-session-failure-toast";
import { listNotificationProviders } from "@/lib/api";

beforeEach(() => {
  vi.clearAllMocks();
  mockNotification = null;
  mockProviders = [];
  mockProvidersLoaded = true;
  delete (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
});

describe("useSessionFailureToast native delivery", () => {
  it("shows toast when notification is set", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "boom" };
    renderHook(() => useSessionFailureToast());

    expect(mockToast).toHaveBeenCalledWith({
      title: "Task failed to start",
      description: "boom",
      variant: "error",
    });
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("also emits one native failure notification when the local preference allows it", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    mockProviders = [
      {
        id: "local",
        type: "local",
        enabled: true,
        events: [WAITING_EVENT],
      },
    ];
    mockNotification = { sessionId: "s-native", taskId: "t-1", message: "boom" };

    const { rerender } = renderHook(() => useSessionFailureToast());
    rerender();

    expect(invoke).toHaveBeenCalledTimes(1);
    expect(invoke).toHaveBeenCalledWith("show_native_notification", {
      request: {
        eventId: "session.failed:s-native",
        title: "Task failed to start",
        body: "boom",
        taskId: "t-1",
        sessionId: "s-native",
      },
    });
    expect(mockToast).toHaveBeenCalledTimes(1);
  });

  it("keeps the failure toast but skips native delivery when the local preference is disabled", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    mockProviders = [
      {
        id: "local",
        type: "local",
        enabled: false,
        events: [WAITING_EVENT],
      },
    ];
    mockNotification = { sessionId: "s-disabled", taskId: "t-1", message: "boom" };

    renderHook(() => useSessionFailureToast());

    expect(mockToast).toHaveBeenCalledTimes(1);
    expect(invoke).not.toHaveBeenCalled();
  });

  it("loads the local preference before native delivery when settings have not been opened", async () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    mockProvidersLoaded = false;
    vi.mocked(listNotificationProviders).mockResolvedValue({
      providers: [
        {
          id: "local",
          name: "Desktop Notifications",
          type: "local",
          config: {},
          enabled: true,
          events: [WAITING_EVENT],
          created_at: "2026-07-15T00:00:00Z",
          updated_at: "2026-07-15T00:00:00Z",
        },
      ],
      events: [WAITING_EVENT],
      apprise_available: false,
    });
    mockNotification = { sessionId: "s-lazy", taskId: "t-1", message: "boom" };

    renderHook(() => useSessionFailureToast());

    await waitFor(() => expect(invoke).toHaveBeenCalledTimes(1));
  });
});

describe("useSessionFailureToast", () => {
  it("does not show toast when notification is null", () => {
    mockNotification = null;
    renderHook(() => useSessionFailureToast());

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).not.toHaveBeenCalled();
  });

  it("deduplicates toasts for the same sessionId across rerenders", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "first" };
    const { rerender } = renderHook(() => useSessionFailureToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();
    mockClearNotification.mockClear();

    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "duplicate" };
    rerender();

    expect(mockToast).not.toHaveBeenCalled();
    expect(mockClearNotification).toHaveBeenCalledWith(null);
  });

  it("shows toast for a different sessionId", () => {
    mockNotification = { sessionId: "s-1", taskId: "t-1", message: "first" };
    const { rerender } = renderHook(() => useSessionFailureToast());
    expect(mockToast).toHaveBeenCalledTimes(1);

    mockToast.mockClear();

    mockNotification = { sessionId: "s-2", taskId: "t-1", message: "second" };
    rerender();

    expect(mockToast).toHaveBeenCalledWith({
      title: "Task failed to start",
      description: "second",
      variant: "error",
    });
  });
});
