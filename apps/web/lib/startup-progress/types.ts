/**
 * Wire shapes for GET /ready's `startup` field, mirroring
 * apps/backend/internal/startup's Snapshot/StepSnapshot JSON tags
 * (AC-PLATFORM-STARTUP-PROGRESS-002/003).
 */

export type StartupPhase =
  | "opening_database"
  | "backing_up_database"
  | "applying_migrations"
  | "initializing_services"
  | "recovering_sessions"
  | "ready";

export type StartupMeasure = "opaque" | "counting" | "counted";

export type StartupUnit = "rows" | "messages" | "turns" | "sessions" | "bytes" | "stores";

export type StartupStepSnapshot = {
  id: string;
  label_key: string;
  measure: StartupMeasure;
  unit: StartupUnit;
  elapsed_ms: number;
  done?: number;
  total?: number;
  rate_per_second?: number;
  eta_ms?: number;
  since_advance_ms?: number;
  stalled: boolean;
};

export type StartupSnapshot = {
  phase: StartupPhase;
  boot: number;
  seq: number;
  elapsed_ms: number;
  phase_elapsed_ms: number;
  step?: StartupStepSnapshot;
};
