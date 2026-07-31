import type { StorageQuarantineEntry } from "@/lib/types/system";

export function quarantineDeleteAfter(entry: StorageQuarantineEntry): Date {
  return new Date(entry.delete_after);
}

export function isQuarantineEligible(entry: StorageQuarantineEntry, now = new Date()): boolean {
  return quarantineDeleteAfter(entry).getTime() <= now.getTime();
}

export function quarantineCounts(entries: StorageQuarantineEntry[], now = new Date()) {
  return entries.reduce(
    (counts, entry) => {
      if (isQuarantineEligible(entry, now)) {
        counts.eligible += 1;
        counts.eligibleBytes += entry.size_bytes;
      } else {
        counts.protected += 1;
        counts.protectedBytes += entry.size_bytes;
      }
      return counts;
    },
    { eligible: 0, eligibleBytes: 0, protected: 0, protectedBytes: 0 },
  );
}

export function formatQuarantineDeadline(entry: StorageQuarantineEntry): string {
  return quarantineDeleteAfter(entry).toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}
