import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NotificationProvider } from "@/lib/types/http";
import { NotificationsSettings } from "./notifications-settings";
import { SettingsSaveProvider } from "./settings-save-provider";

const mocks = vi.hoisted(() => ({
  createNotificationProvider: vi.fn(),
  deleteNotificationProvider: vi.fn(),
  testNotificationProvider: vi.fn(),
  updateNotificationProvider: vi.fn(),
  setNotificationProviders: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  createNotificationProvider: mocks.createNotificationProvider,
  deleteNotificationProvider: mocks.deleteNotificationProvider,
  testNotificationProvider: mocks.testNotificationProvider,
  updateNotificationProvider: mocks.updateNotificationProvider,
}));

// Identities must be stable: the hydration effect in `useNotificationsState`
// compares them by reference, so a fresh array per render re-hydrates forever.
const PROVIDERS: NotificationProvider[] = [];
const EVENTS = ["session.turn_finished"];

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: PROVIDERS,
    events: EVENTS,
    appriseAvailable: false,
    loaded: true,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ setNotificationProviders: mocks.setNotificationProviders }),
}));

vi.mock("@/lib/desktop/native-notification-client", () => ({
  nativeNotifications: {
    isAvailable: () => false,
    permission: { get: vi.fn(), request: vi.fn() },
  },
}));

afterEach(cleanup);

describe("NotificationsSettings", () => {
  it("renders the Apprise install notice as one sentence around its link", () => {
    // <Trans> addresses the anchor by child index, and a drifted index renders
    // duplicated fragments with an empty tag rather than failing (docs/i18n.md).
    const { container } = render(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );

    const anchor = container.querySelector("a");
    expect(anchor?.textContent).toBe("View installation instructions");
    expect(anchor?.getAttribute("href")).toBe(
      "https://github.com/caronc/apprise?tab=readme-ov-file#installation",
    );
    expect(anchor?.parentElement?.textContent).toBe(
      "Apprise is not installed yet. You can add it later to enable remote notifications. " +
        "View installation instructions.",
    );
  });
});
