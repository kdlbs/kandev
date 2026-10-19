export const STARTUP_TIMINGS = [
  {
    id: "database.backup",
    label_key: "startup.step.database_backup",
    activation_seq: 1,
    phase: "backing_up_database",
    elapsed_ms: 82,
    completed: true,
  },
  {
    id: "startup.queue_reconciliation",
    label_key: "startup.step.queue_reconciliation",
    activation_seq: 2,
    phase: "recovering_sessions",
    elapsed_ms: 1200,
    completed: false,
  },
];

export const LONG_STARTUP_TIMINGS = Array.from({ length: 32 }, (_, index) => ({
  ...STARTUP_TIMINGS[index % 2],
  activation_seq: index + 1,
  elapsed_ms: 82 + index,
}));
