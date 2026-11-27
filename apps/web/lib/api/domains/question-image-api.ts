import { ApiError, fetchBlob } from "../client";

export async function fetchQuestionImage(
  pendingId: string,
  questionId: string,
  imageId: string,
  signal: AbortSignal,
): Promise<Blob> {
  const path = `/api/v1/clarification/${encodeURIComponent(pendingId)}/images/${encodeURIComponent(imageId)}?question_id=${encodeURIComponent(questionId)}`;
  const blob = await fetchBlob(path, { cache: "no-store", init: { signal, redirect: "error" } });
  if (blob.size > 10 * 1024 * 1024 || !["image/png", "image/jpeg"].includes(blob.type)) {
    // i18n-exempt: private error code; components render translated copy.
    throw new Error("invalid_image");
  }
  return blob;
}

export function questionImageErrorCode(error: unknown): string {
  if (
    error instanceof ApiError &&
    error.body &&
    typeof error.body === "object" &&
    "code" in error.body
  ) {
    return String(error.body.code);
  }
  return "unavailable";
}
