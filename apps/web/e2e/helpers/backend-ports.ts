import type { WorkerInfo } from "@playwright/test";

export function backendPorts(
  offset: number,
  worker: Pick<WorkerInfo, "workerIndex" | "parallelIndex">,
) {
  if (!Number.isInteger(offset) || offset < 0 || offset > 29) {
    throw new Error(`E2E port offset must be an integer 0-29, got: ${offset}`);
  }
  const slot = worker.parallelIndex;
  if (!Number.isInteger(slot) || slot < 0 || slot > 4) {
    throw new Error(`E2E parallel slot must be an integer 0-4, got: ${slot}`);
  }
  // Replacement workers reuse a slot after its previous fixture has stopped.
  // workerIndex increases across failures and projects and cannot own a port.
  const backendPort = 18080 + offset + slot * 30;
  const standaloneAgentctlPort = backendPort + 1000;
  const agentctlPortBase = 30001 + offset * 1000 + slot * 200;
  return {
    backendPort,
    standaloneAgentctlPort,
    agentctlPortBase,
    agentctlPortMax: agentctlPortBase + 199,
  };
}
