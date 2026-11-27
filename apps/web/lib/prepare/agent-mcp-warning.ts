import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

/**
 * Returns true when an imported MCP verification step failed specifically
 * because authentication is required, which should be presented as an
 * actionable warning rather than a fatal error when overall preparation
 * succeeds.
 */
export function isAgentMcpAuthWarning(
  step: Pick<PrepareStepInfo, "kind" | "status" | "failureCode">,
): boolean {
  return (
    step.kind === "agent_mcp_verification" &&
    step.status === "failed" &&
    step.failureCode === "authentication_required"
  );
}
