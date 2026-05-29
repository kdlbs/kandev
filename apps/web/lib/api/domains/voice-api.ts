import { ApiError, type ApiRequestOptions } from "../client";
import { getBackendConfig } from "@/lib/config";

export type TranscribeResponse = {
  text: string;
};

/**
 * POST audio to the backend Whisper fallback. Returns the transcribed text.
 *
 * Throws ApiError on non-2xx. Two statuses are meaningful to the caller:
 *   - 503: server has no API key configured — the hook should treat the
 *     Whisper fallback as unavailable and surface a clean message.
 *   - any other non-2xx: transient error — show a generic toast.
 */
export async function transcribeAudio(
  blob: Blob,
  filename: string,
  options?: ApiRequestOptions,
): Promise<TranscribeResponse> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const formData = new FormData();
  formData.append("audio", blob, filename);

  // Do NOT set Content-Type: the browser sets multipart/form-data with the
  // correct boundary automatically when given a FormData body. Spread caller
  // init *first* so method/body always win — otherwise a caller passing
  // `init: { method: "GET" }` (or a stale body) would silently break the upload.
  const response = await fetch(`${baseUrl}/api/v1/transcribe`, {
    ...options?.init,
    method: "POST",
    body: formData,
  });

  if (!response.ok) {
    let body: unknown = null;
    try {
      body = await response.json();
    } catch {
      // body remains null
    }
    let message = `Transcription failed: ${response.status} ${response.statusText}`;
    if (body && typeof body === "object" && "error" in body) {
      const errVal = (body as { error?: unknown }).error;
      if (typeof errVal === "string") message = errVal;
    }
    throw new ApiError(message, response.status, body);
  }

  return (await response.json()) as TranscribeResponse;
}
