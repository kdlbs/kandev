export type ParkedTriple = {
  parked?: boolean;
  epoch?: number;
  revision?: number;
};

/**
 * D1's (parked_epoch, revision) lexicographic discard rule (AC-39, AC-49): a
 * strictly higher epoch always wins regardless of revision; within one
 * epoch, discard a strictly lower revision. Pure value comparison — every
 * carrier this rule protects (WS frame, REST DTO, boot payload) resolves
 * through this one function so the rule cannot drift between transports.
 *
 * No existing revision to compare against (first observation) accepts the
 * incoming triple outright. This does not handle field-omission semantics —
 * a carrier where a field can be genuinely absent from a partial frame (a WS
 * event) must special-case that before calling this; a REST DTO always
 * carries all three fields together, so REST callers can call this directly.
 */
export function resolveParkedTriple(existing: ParkedTriple, incoming: ParkedTriple): ParkedTriple {
  if (existing.revision === undefined) {
    return incoming;
  }

  const incomingEpoch = incoming.epoch ?? 0;
  const existingEpoch = existing.epoch ?? 0;
  const incomingRevision = incoming.revision ?? 0;
  const incomingIsCurrent =
    incomingEpoch > existingEpoch ||
    (incomingEpoch === existingEpoch && incomingRevision >= existing.revision);

  return incomingIsCurrent ? incoming : existing;
}
