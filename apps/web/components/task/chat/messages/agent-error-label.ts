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
 */

export const ENVIRONMENT_SETUP_FAILED_LABEL = "Environment setup failed";

// Signatures emitted while preparing the workspace / launching the executor,
// before the agent process is meaningfully running. Sourced from the Go
// backend: `environment preparation failed:` (manager_launch.go), the launch
// race ("already has an agent running" / "race resolved during register"),
// container launch failures, and branch/fresh-branch prep errors.
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
 * Resolves the label shown in the FAILED status banner. Returns the
 * "Environment setup failed" label only for genuine setup failures; otherwise
 * the caller's fallback (the generic "Agent has encountered an error").
 */
export function resolveAgentErrorLabel(
  errorMessage: string | undefined | null,
  fallbackLabel: string,
): string {
  return isEnvironmentSetupError(errorMessage) ? ENVIRONMENT_SETUP_FAILED_LABEL : fallbackLabel;
}
