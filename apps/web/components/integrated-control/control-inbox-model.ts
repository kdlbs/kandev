import type { ClarificationInboxBundle, InboxRecoveryBlock } from "@/lib/types/clarification-inbox";
import type { FailedInboxRow } from "@/lib/types/failed-inbox";

export type ControlInboxReadStatus = "ready" | "denied" | "unsupported" | "error";

/** One workspace's Needs-you reads; a section that failed carries its status, never zero rows. */
export type ControlInboxWorkspaceRead = {
  workspaceId: string;
  answerable: ControlInboxSection<ClarificationInboxBundle>;
  earlier: ControlInboxSection<ClarificationInboxBundle>;
  recovery: ControlInboxSection<InboxRecoveryBlock>;
  failed: ControlInboxSection<FailedInboxRow>;
};

export type ControlInboxSection<T> = {
  status: ControlInboxReadStatus;
  items: T[];
  truncated: boolean;
};

export type ControlInboxSectionKey = "answerable" | "earlier" | "recovery" | "failed";
export const CONTROL_INBOX_SECTIONS: ControlInboxSectionKey[] = [
  "answerable",
  "earlier",
  "recovery",
  "failed",
];

export type ControlInboxGroup<T> = {
  workspaceId: string;
  status: ControlInboxReadStatus;
  items: T[];
  truncated: boolean;
};

export type ControlInboxCounts = {
  /** Questions answerable in place plus earlier-turn questions plus recovery decisions. */
  pending: number;
  answerable: number;
  earlier: number;
  recovery: number;
  failed: number;
  /** True when any section of any workspace could not be read or was truncated. */
  partial: boolean;
};

export type WorkspaceInboxFigures = {
  answerable: number;
  earlier: number;
  recovery: number;
  /** The oldest waiting item's instant, for the attention row's duration. */
  oldest?: string;
};

/** Per-section groups in workspace order, keeping only workspaces with rows or a failed read. */
export function groupInboxSection<T extends { created_at?: string; failure_instant?: string }>(
  reads: ControlInboxWorkspaceRead[],
  pick: (read: ControlInboxWorkspaceRead) => ControlInboxSection<T>,
): ControlInboxGroup<T>[] {
  return reads
    .map((read) => ({ workspaceId: read.workspaceId, ...pick(read) }))
    .filter((group) => group.status !== "ready" || group.items.length > 0)
    .map((group) => ({ ...group, items: [...group.items].sort(byCreatedAt) }));
}

function itemInstant(item: { created_at?: string; failure_instant?: string }): number {
  const time = Date.parse(item.created_at ?? item.failure_instant ?? "");
  return Number.isNaN(time) ? Number.MAX_SAFE_INTEGER : time;
}

function byCreatedAt(
  a: { created_at?: string; failure_instant?: string },
  b: { created_at?: string; failure_instant?: string },
): number {
  return itemInstant(a) - itemInstant(b);
}

export function countInbox(reads: ControlInboxWorkspaceRead[]): ControlInboxCounts {
  const counts = { pending: 0, answerable: 0, earlier: 0, recovery: 0, failed: 0, partial: false };
  for (const read of reads)
    for (const key of CONTROL_INBOX_SECTIONS) {
      const section = read[key];
      counts[key] += section.items.length;
      if (section.status !== "ready" || section.truncated) counts.partial = true;
    }
  counts.pending = counts.answerable + counts.earlier + counts.recovery;
  return counts;
}

/** What each workspace is waiting on a person for; workspaces with nothing are omitted. */
export function workspaceInboxFigures(
  reads: ControlInboxWorkspaceRead[],
): Record<string, WorkspaceInboxFigures> {
  const result: Record<string, WorkspaceInboxFigures> = {};
  for (const read of reads) {
    const figures = {
      answerable: read.answerable.items.length,
      earlier: read.earlier.items.length,
      recovery: read.recovery.items.length,
    };
    if (figures.answerable + figures.earlier + figures.recovery === 0) continue;
    const instants = [...read.answerable.items, ...read.earlier.items, ...read.recovery.items]
      .map((item) => item.created_at)
      .filter((value) => !Number.isNaN(Date.parse(value)))
      .sort((a, b) => Date.parse(a) - Date.parse(b));
    result[read.workspaceId] = { ...figures, oldest: instants[0] };
  }
  return result;
}
