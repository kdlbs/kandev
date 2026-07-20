import { describe, expect, it } from "vitest";
import type { BootPayload } from "@/src/boot-payload";
import { applyBrowserDemoDefaults } from "./install";

describe("applyBrowserDemoDefaults", () => {
  it("disables preview-on-click before the browser demo mounts", () => {
    const payload = {
      version: 1,
      initialState: {
        userSettings: {
          enablePreviewOnClick: true,
          loaded: true,
        },
      },
    } as BootPayload;

    const result = applyBrowserDemoDefaults(payload);

    expect(result.initialState?.userSettings?.enablePreviewOnClick).toBe(false);
    expect(result.initialState?.userSettings?.loaded).toBe(true);
    expect(payload.initialState?.userSettings?.enablePreviewOnClick).toBe(true);
  });

  it("leaves payloads without user settings unchanged", () => {
    const payload: BootPayload = { version: 1, initialState: {} };

    expect(applyBrowserDemoDefaults(payload)).toBe(payload);
  });
});
