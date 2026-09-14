import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@kandev/ui/sonner";
import { toast, type ToasterProps } from "sonner";
import { AppThemeProvider, useTheme } from "./app-theme";

const TOAST_SELECTOR = "[data-sonner-toast]";

function ThemeControls() {
  const { previewTheme, restoreTheme } = useTheme();
  return (
    <>
      <button onClick={() => previewTheme("light")}>Preview light</button>
      <button onClick={restoreTheme}>Discard preview</button>
    </>
  );
}

function mountToaster(theme: string, systemDark = false, props: ToasterProps = {}) {
  window.localStorage.setItem("theme", theme);
  const media = Object.assign(new EventTarget(), { matches: systemDark });
  vi.stubGlobal("matchMedia", () => media);
  const result = render(
    <AppThemeProvider>
      <Toaster richColors {...props} />
      <ThemeControls />
    </AppThemeProvider>,
  );
  return {
    ...result,
    async setSystemDark(dark: boolean) {
      await act(async () => {
        media.matches = dark;
        media.dispatchEvent(new Event("change"));
      });
    },
  };
}

async function showNotification(title = "Plugin installed") {
  const action = vi.fn();
  await act(async () => {
    toast.success(title, { duration: Infinity, action: { label: "Open plugin", onClick: action } });
  });
  const text = await screen.findByText(title);
  const notification = text.closest(TOAST_SELECTOR);
  expect(notification).not.toBeNull();
  return { notification, action };
}

async function expectToastTheme(theme: "light" | "dark") {
  await waitFor(() => {
    expect(document.querySelector("[data-sonner-toaster]")?.getAttribute("data-sonner-theme")).toBe(
      theme,
    );
  });
}

beforeEach(() => {
  document.documentElement.className = "";
  window.localStorage.clear();
});

afterEach(async () => {
  await act(async () => {
    toast.dismiss();
  });
  cleanup();
  document.documentElement.className = "";
  window.localStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Sonner application theme synchronization", () => {
  // @covers AC-UI-TOAST-THEME-001.1, AC-UI-TOAST-THEME-001.2
  it.each([
    { theme: "dark", systemDark: false, expected: "dark" },
    { theme: "system", systemDark: true, expected: "dark" },
    { theme: "light", systemDark: true, expected: "light" },
    { theme: "system", systemDark: false, expected: "light" },
  ] as const)(
    "uses $expected at cold load with theme=$theme and systemDark=$systemDark",
    async ({ theme, systemDark, expected }) => {
      mountToaster(theme, systemDark);
      expect(document.documentElement.className).toBe(expected);
      await showNotification();
      await expectToastTheme(expected);
    },
  );

  // @covers AC-UI-TOAST-THEME-001.3
  it("preserves an existing notification through preview and discard", async () => {
    mountToaster("dark");
    const { notification, action } = await showNotification();

    fireEvent.click(screen.getByRole("button", { name: "Preview light" }));
    await expectToastTheme("light");
    expect(window.localStorage.getItem("theme")).toBe("dark");

    fireEvent.click(screen.getByRole("button", { name: "Discard preview" }));
    await expectToastTheme("dark");
    expect(document.querySelectorAll(TOAST_SELECTOR)).toHaveLength(1);
    expect(screen.getByText("Plugin installed").closest(TOAST_SELECTOR)).toBe(notification);
    fireEvent.click(screen.getByRole("button", { name: "Open plugin" }));
    expect(action).toHaveBeenCalledOnce();
  });

  // @covers AC-UI-TOAST-THEME-001.3
  it("updates existing and subsequent notifications when the system theme changes", async () => {
    const { setSystemDark } = mountToaster("system");
    const { notification } = await showNotification();
    await expectToastTheme("light");

    await setSystemDark(true);
    await expectToastTheme("dark");
    expect(screen.getByText("Plugin installed").closest(TOAST_SELECTOR)).toBe(notification);
    await showNotification("Plugin updated");
    await expectToastTheme("dark");
    expect(document.querySelectorAll(TOAST_SELECTOR)).toHaveLength(2);

    await setSystemDark(false);
    await expectToastTheme("light");
  });

  it("preserves an explicit wrapper theme override", async () => {
    const { setSystemDark } = mountToaster("system", false, { theme: "light" });
    await showNotification();
    await setSystemDark(true);
    expect(document.documentElement.className).toBe("dark");
    await expectToastTheme("light");
  });

  it("stops observing the document after unmount", async () => {
    const observe = vi.spyOn(MutationObserver.prototype, "observe");
    const { unmount } = mountToaster("light");
    await showNotification();
    const rootIndex = observe.mock.calls.findIndex(
      ([target]) => target === document.documentElement,
    );
    expect(rootIndex).toBeGreaterThanOrEqual(0);
    const observer = observe.mock.contexts[rootIndex] as MutationObserver;

    unmount();
    document.documentElement.className = "dark";
    expect(observer.takeRecords()).toEqual([]);
  });
});
