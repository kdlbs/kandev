/**
 * Re-export of shared cache-data types for the session-runtime TQ bridge.
 * Source of truth is session-runtime query-options; this file keeps imports
 * clean for consumers that shouldn't depend on query-options directly.
 */
export type {
  GitStatusData,
  SessionModeData,
  SessionModelsData,
} from "@/lib/query/query-options/session-runtime";
