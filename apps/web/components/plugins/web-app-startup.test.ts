import { describe, expect, it, vi } from "vitest";
import {
  createWebAppStartupProbe,
  parseWebAppStartupResult,
  WEB_APP_STARTUP_PROBE_TYPE,
  WEB_APP_STARTUP_RESULT_TYPE,
  WEB_APP_STARTUP_VERSION,
} from "./web-app-startup";

vi.mock("@/lib/utils", () => ({
  generateUUID: () => "startup-nonce",
}));

const startupNonce = "startup-nonce";

describe("web app startup protocol", () => {
  it("creates a versioned probe with a bounded nonce", () => {
    expect(createWebAppStartupProbe(startupNonce)).toEqual({
      type: WEB_APP_STARTUP_PROBE_TYPE,
      version: WEB_APP_STARTUP_VERSION,
      nonce: startupNonce,
    });
  });

  it("accepts only the current nonce and closed result vocabulary", () => {
    expect(
      parseWebAppStartupResult(
        {
          type: WEB_APP_STARTUP_RESULT_TYPE,
          version: WEB_APP_STARTUP_VERSION,
          nonce: startupNonce,
          result: "ready",
        },
        startupNonce,
      ),
    ).toMatchObject({ result: "ready", nonce: startupNonce });
    expect(
      parseWebAppStartupResult(
        {
          type: WEB_APP_STARTUP_RESULT_TYPE,
          version: WEB_APP_STARTUP_VERSION,
          nonce: startupNonce,
          result: "failed",
          code: "context_unavailable",
        },
        startupNonce,
      ),
    ).toMatchObject({ result: "failed", code: "context_unavailable" });

    for (const value of [
      { type: WEB_APP_STARTUP_RESULT_TYPE, version: 2, nonce: startupNonce, result: "ready" },
      { type: WEB_APP_STARTUP_RESULT_TYPE, version: 1, nonce: "old", result: "ready" },
      {
        type: WEB_APP_STARTUP_RESULT_TYPE,
        version: WEB_APP_STARTUP_VERSION,
        nonce: startupNonce,
        result: "failed",
        code: "secret_leaked",
      },
      {
        type: WEB_APP_STARTUP_RESULT_TYPE,
        version: WEB_APP_STARTUP_VERSION,
        nonce: startupNonce,
        result: "ready",
        error: "sensitive details",
      },
    ]) {
      expect(parseWebAppStartupResult(value, startupNonce)).toBeNull();
    }
  });
});
