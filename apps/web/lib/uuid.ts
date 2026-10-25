let uuidFallbackCounter = 0;
let uuidFallbackContextCounter = 0;

function createUuidFallbackContext(): string {
  const entropy = new Uint32Array(2);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    try {
      crypto.getRandomValues(entropy);
      return entropy[0].toString(16).padStart(8, "0");
    } catch {
      // Fall through to the non-cryptographic legacy-environment fallback.
    }
  }

  const monotonic = typeof performance !== "undefined" ? Math.floor(performance.now() * 1000) : 0;
  const counter = uuidFallbackContextCounter++;
  return ((Date.now() ^ monotonic ^ counter) >>> 0).toString(16).padStart(8, "0");
}

// Keep a per-module component so two tabs generating IDs in the same
// millisecond do not share the timestamp/counter prefix. Prefer Web Crypto
// entropy when available; old environments get a monotonic best-effort ID.
const uuidFallbackContext = createUuidFallbackContext();

/**
 * Generate a UUID. Falls back to crypto.getRandomValues when randomUUID is
 * unavailable (e.g., HTTP on non-localhost), then to a UUID-shaped fallback
 * with a per-context component for legacy environments without any Web Crypto API.
 */
export function generateUUID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }

  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }

  const context = uuidFallbackContext;
  const timestamp = Date.now().toString(16).padStart(12, "0").slice(-12);
  const counter = (uuidFallbackCounter++ >>> 0).toString(16).padStart(8, "0");
  const chars = `${context}${timestamp}${counter}`.padEnd(32, "0").slice(0, 32).split("");
  chars[12] = "4";
  chars[16] = "8";
  const hex = chars.join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
