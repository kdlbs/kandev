import type { Task } from "@/lib/types/http";

export const PORT_FORWARDING_METADATA_KEY = "port_forwarding_enabled";

export function isPortForwardingEnabled(
  metadata: Task["metadata"] | Record<string, unknown> | null | undefined,
): boolean {
  return metadata?.[PORT_FORWARDING_METADATA_KEY] === true;
}

export function canUsePortForwarding(params: {
  sessionId?: string | null;
  isAgentctlReady: boolean;
}): boolean {
  return Boolean(params.sessionId && params.isAgentctlReady);
}
