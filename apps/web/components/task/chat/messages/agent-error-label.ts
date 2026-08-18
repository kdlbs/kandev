/**
 * Distinguishes genuine workspace/environment-setup failures from downstream
 * agent / API errors so the FAILED status banner shows an accurate label.
 *
 * "Environment setup failed" is a frontend-only label — the backend never
 * emits it. Historically the banner showed it for *any* failure with an error
 * message, which mislabeled agent/API errors (auth, rate limit, the
 * thinking-blocks 400, etc.) as setup failures. We now only claim setup
 * failure when the message matches a known setup signature; everything else
 * keeps the accurate generic FAILED label and surfaces the raw message in the
 * expandable details.
 *
 * Labels travel as catalog keys, not resolved copy: this module is imported at
 * module scope, so a `t()` here would freeze at the boot locale.
 */

export const ENVIRONMENT_SETUP_FAILED_KEY = "task:environmentSetupFailed";

// Signatures emitted while preparing the workspace / launching the executor,
// before the agent process is meaningfully running. Sourced from the Go
// backend: `environment preparation failed:` (manager_launch.go), the launch
// race ("already has an agent running" / "race resolved during register"),
// container launch failures, and branch/fresh-branch prep errors.
// i18n-exempt: substrings matched against backend error text, not copy.
const SETUP_FAILURE_SIGNATURES = [
  "environment preparation failed",
  "failed to launch container",
  "already has an agent running",
  "race resolved during register",
  "failed to prepare",
];

export function isEnvironmentSetupError(message: string | undefined | null): boolean {
  if (!message) return false;
  const lower = message.toLowerCase();
  return SETUP_FAILURE_SIGNATURES.some((sig) => lower.includes(sig));
}

/**
 * Resolves the catalog key for the label shown in the FAILED status banner.
 * Returns the "Environment setup failed" key only for genuine setup failures;
 * otherwise the caller's fallback key (the generic "Agent has encountered an
 * error").
 */
export function resolveAgentErrorLabelKey(
  errorMessage: string | undefined | null,
  fallbackKey: string,
): string {
  return isEnvironmentSetupError(errorMessage) ? ENVIRONMENT_SETUP_FAILED_KEY : fallbackKey;
}
