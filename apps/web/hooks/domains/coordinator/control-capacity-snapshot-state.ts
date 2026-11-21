import type { SessionCapacityObservation } from "@/lib/types/system";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { ControlReadStatus } from "./use-control-resource";

type CapacitySnapshot = {
  capacity?: SessionCapacityObservation;
  lastConfirmedCapacity?: SessionCapacityObservation;
  capacityReceivedAt?: string;
};

function confirmedCapacity(observation?: SessionCapacityObservation) {
  const at = parseStrictRfc3339Timestamp(observation?.observed_at ?? "");
  return observation?.known &&
    at !== null &&
    Number(at / BigInt(1_000_000)) <= Date.now() + 5_000 &&
    Number.isSafeInteger(observation.in_use) &&
    Number(observation.in_use) >= 0
    ? observation
    : undefined;
}

export function retainCapacity(
  previous: CapacitySnapshot | undefined,
  capacity: SessionCapacityObservation | undefined,
  status: ControlReadStatus,
) {
  const retained = status === "error";
  const allowed = status === "ready" || retained;
  return {
    capacity: retained ? previous?.capacity : capacity,
    capacityStatus: status,
    capacityReceivedAt: retained ? previous?.capacityReceivedAt : new Date().toISOString(),
    lastConfirmedCapacity:
      confirmedCapacity(capacity) ?? (allowed ? previous?.lastConfirmedCapacity : undefined),
  };
}
