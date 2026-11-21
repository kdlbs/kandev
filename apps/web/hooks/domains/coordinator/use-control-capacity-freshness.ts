import { useOptionalAppStore } from "@/components/state-provider";
import { useNow } from "@/hooks/use-now";
import { useIntegratedControl } from "@/components/integrated-control/integrated-control-provider";
import type { SessionCapacityObservation } from "@/lib/types/system";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { LAUNCH_QUEUE_CAPACITY_STALE_AFTER_MS } from "@/lib/tasks/launch-queue-view-model";

export function useControlCapacityFreshness(observation?: SessionCapacityObservation) {
  const control = useIntegratedControl();
  const now = useNow(5_000);
  const connected = useOptionalAppStore((state) => state.connection.status === "connected", true);
  const parsedAt = parseStrictRfc3339Timestamp(observation?.observed_at ?? "");
  const sampledAt = parsedAt === null ? NaN : Number(parsedAt / BigInt(1_000_000));
  const receivedAt = Date.parse(control?.data?.receivedAt ?? "");
  const currentTime = Number.isFinite(receivedAt) ? Math.max(now, receivedAt) : now;
  const validTime = Number.isFinite(sampledAt) && sampledAt <= currentTime + 5_000;
  return {
    validTime,
    known: observation?.known === true && validTime,
    stale: capacityIsStale(connected, capacityReadPending(control), currentTime - sampledAt),
  };
}

/** A failed read makes the shown capacity stale; a read in progress does not. */
function capacityReadPending(control: ReturnType<typeof useIntegratedControl>) {
  return Boolean(control?.error || control?.data?.capacityStatus === "error");
}

function capacityIsStale(connected: boolean, refreshing: boolean, age: number) {
  return !connected || refreshing || age >= LAUNCH_QUEUE_CAPACITY_STALE_AFTER_MS;
}
