import type { PermissionPresentation } from "@/lib/types/permission";

/**
 * Metadata key mirrored from the backend models.PermissionPresentationMetadataKey.
 */
export const PERMISSION_PRESENTATION_METADATA_KEY = "permission_presentation";

const KNOWN_PRESENTATIONS: ReadonlySet<string> = new Set([
  "automatic",
  "interactive",
  "unavailable",
]);

/**
 * Normalizes a persisted presentation value. Missing and unknown values retain
 * the legacy interactive behavior so older rows and newer writers stay safe.
 */
export function parsePermissionPresentation(value: unknown): PermissionPresentation {
  return typeof value === "string" && KNOWN_PRESENTATIONS.has(value)
    ? (value as PermissionPresentation)
    : "interactive";
}

/**
 * Reads presentation intent from message metadata. The classifier is
 * dependency-neutral so transcript filtering, rendering, and attention
 * selectors all make the same decision.
 */
export function permissionPresentationFromMetadata(
  metadata: Record<string, unknown> | null | undefined,
): PermissionPresentation {
  return parsePermissionPresentation(metadata?.[PERMISSION_PRESENTATION_METADATA_KEY]);
}

/** Whether the request needs a person to answer it. */
export function permissionRequestsInput(presentation: PermissionPresentation): boolean {
  return presentation === "interactive";
}

/** Automatic decisions render ordinary tool progress, never input controls. */
export function isAutomaticPermissionPresentation(presentation: PermissionPresentation): boolean {
  return presentation === "automatic";
}

/** Unavailable decisions render non-actionable feedback, never input controls. */
export function isUnavailablePermissionPresentation(presentation: PermissionPresentation): boolean {
  return presentation === "unavailable";
}
