import { describe, expect, it } from "vitest";
import {
  isBoundedCssColor,
  resolveWebAppAppearance,
  WEB_APP_APPEARANCE_TOKEN_KEYS,
  WEB_APP_APPEARANCE_TYPE,
  WEB_APP_APPEARANCE_VERSION,
} from "./web-app-appearance";

describe("web app appearance protocol", () => {
  it("resolves only the documented semantic token envelope", () => {
    document.documentElement.classList.remove("dark");
    document.documentElement.style.setProperty("--background", "#123456");
    document.documentElement.style.setProperty("--foreground", "oklch(0.2 0 0)");

    const message = resolveWebAppAppearance(document);

    expect(message).toEqual({
      type: WEB_APP_APPEARANCE_TYPE,
      version: WEB_APP_APPEARANCE_VERSION,
      mode: "light",
      tokens: expect.objectContaining({
        background: "#123456",
        foreground: "oklch(0.2 0 0)",
      }),
    });
    expect(Object.keys(message)).toEqual(["type", "version", "mode", "tokens"]);
    expect(Object.keys(message.tokens)).toEqual([...WEB_APP_APPEARANCE_TOKEN_KEYS]);
    expect(message.tokens).not.toHaveProperty("workspaceId");
    expect(message.tokens).not.toHaveProperty("runtimeUrl");
  });

  it("uses dark fallbacks for missing or unsafe host values", () => {
    document.documentElement.classList.add("dark");
    document.documentElement.style.setProperty("--background", "url(https://evil.invalid)");
    document.documentElement.style.setProperty("--foreground", "var(--secret)");

    const message = resolveWebAppAppearance(document, "dark");

    expect(message.mode).toBe("dark");
    expect(message.tokens.background).toBe("oklch(0.145 0 0)");
    expect(message.tokens.foreground).toBe("oklch(0.985 0 0)");
  });

  it("bounds serialized color values", () => {
    expect(isBoundedCssColor("oklch(0.5 0.2 240)")).toBe(true);
    expect(isBoundedCssColor("#abc")).toBe(true);
    expect(isBoundedCssColor("url(https://example.com/x)")).toBe(false);
    expect(isBoundedCssColor("red; background: url(x)")).toBe(false);
    expect(isBoundedCssColor("x".repeat(129))).toBe(false);
  });
});
