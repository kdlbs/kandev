import { IconAlertTriangle, IconShieldCheck, IconTerminal2 } from "@tabler/icons-react";
import type { ExecutionStatus } from "@/lib/types/execution";

export const executionStatusKeys = {
  not_configured: "agents:executionNotConfigured",
  configured: "agents:executionConfigured",
  starting: "agents:executionStarting",
  active: "agents:executionActive",
  failed: "agents:executionFailed",
  unknown: "agents:executionUnknown",
  stopped: "agents:executionStopped",
} as const;

export function ExecutionStatusIcon({ status }: { status: ExecutionStatus }) {
  let Icon = IconTerminal2;
  if (status === "active") Icon = IconShieldCheck;
  if (status === "failed") Icon = IconAlertTriangle;
  return <Icon aria-hidden="true" className="size-4 shrink-0" stroke={1.5} />;
}
