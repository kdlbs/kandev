/**
 * Extraction for the reason a task move was refused.
 *
 * Deliberately free of React and of the alert primitives: the banner renders
 * it, but so does the proceed-to-next-step toast in a chat hook, and a plain
 * helper keeps that from dragging component modules into unrelated bundles.
 */

export function getTaskMoveErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
}

/**
 * The detail worth rendering beneath the headline, or null when extraction fell
 * back to the headline itself. Repeating the same sentence twice reads as a
 * rendering bug and tells the user nothing the headline has not already said.
 */
export function getTaskMoveErrorDetail(error: unknown, title: string): string | null {
  const detail = getTaskMoveErrorMessage(error, title);
  return detail === title ? null : detail;
}
