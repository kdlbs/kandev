# ADR-2026-09-10-storage-overlap-attribution: Attribute storage by measured files

**Status:** accepted

**Date:** 2026-09-10

**Area:** backend, frontend

## Context

Storage categories can overlap at directory boundaries. A database backup tree
may contain a Go cache subtree, and a configured cache may move away from the
install's default path. Root-level overlap checks can therefore either double
count files or suppress unrelated files from the total. A cold scan also has a
valid intermediate state in which a source has progress but no measurement yet.

## Decision

Storage scanners shall retain each source's full measured footprint and report
the bytes covered by effective overlap roots at file level. Database
measurements shall expose both `size_bytes` and `counted_size_bytes`; the
counted total shall use the latter. A source with only part of its footprint
overlapping another source remains included with a stable partial-overlap
reason. A source whose entire non-zero footprint overlaps remains excluded.

Attribution roots shall come only from the provider roots used successfully by
the current scan. A historical default path is not an overlap root after an
adopted or otherwise effective path changes.

When a database source is pending or scanning and its measurement is absent,
the Storage rows shall render the source progress. A terminal response without
the measurement remains unknown or unavailable rather than being shown as zero.

## Consequences

The API carries a second byte value for database rows, and the UI can explain
partial attribution without misleading operators. Scanner progress continues
to report the full walked footprint. The overlap calculation is lexical and
read-only; hard-link accounting and host-wide reconciliation remain outside the
storage analysis scope.

## Alternatives Considered

- Exclude a whole row whenever its root overlaps another root. This loses
  distinct files under partially overlapping directories.
- Keep a fixed default cache root as a compatibility safeguard. This suppresses
  database bytes when the effective cache has moved elsewhere.
- Mark every partial overlap as unavailable. This hides a valid measured
  footprint and prevents the total from using the bytes that can be attributed
  safely.
