import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
  rescanApprise: vi.fn(),
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
let appriseAvailable = false;
let appriseRescanPending = false;
let appriseRescanError = false;
let appriseRescanResult: boolean | null = null;

vi.mock("@/hooks/domains/settings/use-notification-providers", () => ({
  useNotificationProviders: () => ({
    providers: PROVIDERS,
    events: EVENTS,
    appriseAvailable,
    loaded: true,
    rescanApprise: mocks.rescanApprise,
    appriseRescanPending,
    appriseRescanError,
    appriseRescanResult,
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

afterEach(() => {
  cleanup();
  appriseAvailable = false;
  appriseRescanPending = false;
  appriseRescanError = false;
  appriseRescanResult = null;
});

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

  // The page body renders inside a `SettingsCard`, whose `Card` base is
  // `text-xs/relaxed`; sibling settings pages keep group headings at `text-sm`
  // and descriptions/tables at `text-xs`. This body used to hard-code
  // `text-base`/`text-sm` throughout, so the whole page read a size larger than
  // its siblings. The `text-2xl` page title above the card is the shared
  // `SettingsPageTemplate` heading and is deliberately out of scope. jsdom has
  // no Tailwind, so assert on the utility classes rather than computed sizes.
  it("keeps the card body within the settings type scale", () => {
    const { container } = render(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );

    const body = container.querySelector('[data-slot="card-content"]');
    expect(body).not.toBeNull();
    const oversized = [...body!.querySelectorAll("[class]")]
      .map((element) => element.getAttribute("class") ?? "")
      // Lookahead, not `(?:\s|$)`: Tailwind's line-height modifier makes the
      // next character `/` (the Card base is literally `text-xs/relaxed`), so a
      // consuming boundary would let `text-xl/relaxed` through.
      .filter((className) => /(?:^|\s)text-(?:base|lg|\d?xl)(?=[\s/]|$)/.test(className));
    expect(oversized).toEqual([]);
  });

  it("rescans Apprise and exposes the fresh detection result", () => {
    const { rerender } = render(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );

    const rescan = screen.getByRole("button", { name: "Rescan Apprise" });
    expect(rescan).toBeTruthy();
    expect(screen.getByText(/Detection runs on the Kandev server/)).toBeTruthy();

    fireEvent.click(rescan);
    expect(mocks.rescanApprise).toHaveBeenCalledOnce();

    appriseRescanPending = true;
    rerender(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );
    expect(
      (screen.getByRole("button", { name: "Checking Apprise" }) as HTMLButtonElement).disabled,
    ).toBe(true);

    appriseAvailable = true;
    appriseRescanPending = false;
    appriseRescanResult = true;
    rerender(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );
    expect(screen.getByText("Apprise detected.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add Apprise Provider" })).toBeTruthy();
  });

  it("keeps the last availability and offers retry feedback after a rescan error", () => {
    const { rerender } = render(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );

    appriseRescanError = true;
    rerender(
      <SettingsSaveProvider>
        <NotificationsSettings />
      </SettingsSaveProvider>,
    );

    expect(screen.getByText("Could not check Apprise. Try again.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Rescan Apprise" })).toBeTruthy();
    expect(screen.getByText(/Apprise is not installed yet/)).toBeTruthy();
  });
});
