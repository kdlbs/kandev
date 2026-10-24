import { expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { refreshThemeInventory } from "./theme-loader";
import { listThemes } from "@/lib/api/domains/themes-api";
import type { ThemeInventory } from "@/lib/theme/theme-types";
vi.mock("@/lib/api/domains/themes-api", () => ({
  listThemes: vi.fn(),
  getThemeDefinition: vi.fn(),
}));
it("retains the latest inventory when older requests finish later", async () => {
  const store = createAppStore();
  let resolveOld!: (value: ThemeInventory) => void;
  vi.mocked(listThemes)
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    )
    .mockResolvedValueOnce({ boot_id: "b", revision: 2, themes: [] });
  const first = refreshThemeInventory(store);
  await refreshThemeInventory(store);
  resolveOld({ boot_id: "b", revision: 1, themes: [] });
  await first;
  expect(store.getState().themeInventory?.revision).toBe(2);
});

it("retains acknowledged inventory after a refresh error", async () => {
  const store = createAppStore();
  store.getState().setThemeInventory({ boot_id: "b", revision: 7, themes: [] });
  vi.mocked(listThemes).mockRejectedValueOnce(new Error("offline"));
  await refreshThemeInventory(store);
  expect(store.getState().themeInventory?.revision).toBe(7);
  expect(store.getState().themeError).toBe("offline");
});
it("ignores in-flight inventory after logout", async () => {
  const store = createAppStore();
  let resolve!: (value: ThemeInventory) => void;
  vi.mocked(listThemes).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const pending = refreshThemeInventory(store);
  store.setState((s) => ({ auth: { ...s.auth, authenticated: false } }));
  resolve({ boot_id: "old", revision: 99, themes: [] });
  await pending;
  expect(store.getState().themeInventory).toBeNull();
});
