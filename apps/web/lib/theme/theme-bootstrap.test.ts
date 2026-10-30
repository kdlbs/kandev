import { afterEach, expect, it, vi } from "vitest";
import { bootstrapTheme } from "./theme-bootstrap";
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  document.documentElement.className = "";
});
it("never treats a stored package reference as trusted palette data", () => {
  localStorage.setItem("theme", "theme:example/night");
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  bootstrapTheme();
  expect(document.documentElement.className).toBe("dark");
});
it("boots when browser storage throws", () => {
  vi.stubGlobal("localStorage", {
    getItem() {
      throw new Error("denied");
    },
  });
  vi.stubGlobal("matchMedia", () => ({ matches: false }));
  bootstrapTheme();
  expect(document.documentElement.className).toBe("light");
});
