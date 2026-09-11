import { generateUUID } from "@/lib/utils";

export const WEB_APP_STARTUP_VERSION = 1;
export const WEB_APP_STARTUP_TIMEOUT_MS = 15_000;
// i18n-exempt: wire protocol identifiers, not user-facing copy.
export const WEB_APP_STARTUP_PROBE_TYPE = "kandev.web_app.startup_probe";
// i18n-exempt: wire protocol identifiers, not user-facing copy.
export const WEB_APP_STARTUP_RESULT_TYPE = "kandev.web_app.startup_result";

const MAX_STARTUP_NONCE_BYTES = 128;

export type WebAppStartupProbe = {
  type: typeof WEB_APP_STARTUP_PROBE_TYPE;
  version: typeof WEB_APP_STARTUP_VERSION;
  nonce: string;
};

export type WebAppStartupResult = {
  type: typeof WEB_APP_STARTUP_RESULT_TYPE;
  version: typeof WEB_APP_STARTUP_VERSION;
  nonce: string;
  result: "ready" | "failed";
  code?: "document_error" | "context_unavailable";
};

export function createWebAppStartupNonce(): string {
  return generateUUID();
}

export function createWebAppStartupProbe(nonce: string): WebAppStartupProbe {
  return {
    type: WEB_APP_STARTUP_PROBE_TYPE,
    version: WEB_APP_STARTUP_VERSION,
    nonce,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isValidNonce(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= MAX_STARTUP_NONCE_BYTES;
}

export function parseWebAppStartupResult(
  value: unknown,
  expectedNonce: string,
): WebAppStartupResult | null {
  if (!isRecord(value)) return null;
  if (
    Object.keys(value).some((key) => !["type", "version", "nonce", "result", "code"].includes(key))
  ) {
    return null;
  }
  if (value.type !== WEB_APP_STARTUP_RESULT_TYPE) return null;
  if (value.version !== WEB_APP_STARTUP_VERSION) return null;
  if (value.nonce !== expectedNonce || !isValidNonce(value.nonce)) return null;
  if (value.result !== "ready" && value.result !== "failed") return null;
  if (value.result === "ready") {
    if ("code" in value) return null;
    return {
      type: WEB_APP_STARTUP_RESULT_TYPE,
      version: WEB_APP_STARTUP_VERSION,
      nonce: value.nonce,
      result: "ready",
    };
  }
  if (value.code !== "document_error" && value.code !== "context_unavailable") return null;
  return {
    type: WEB_APP_STARTUP_RESULT_TYPE,
    version: WEB_APP_STARTUP_VERSION,
    nonce: value.nonce,
    result: "failed",
    code: value.code,
  };
}
