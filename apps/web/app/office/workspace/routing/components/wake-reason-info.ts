import type { WakeReason } from "@/lib/state/slices/office/types";

// WakeReasonCopy describes one row in the wake-reason tier policy
// surface. Shared between the workspace settings card and the
// per-agent override panel so the explanation stays consistent.
export type WakeReasonCopy = {
  id: WakeReason;
  label: string;
  short: string;
  long: string;
  recommendation: string;
};

export const WAKE_REASONS: WakeReasonCopy[] = [
  {
    id: "heartbeat",
    label: "Heartbeat",
    short:
      "Periodic check-in your agent runs on a schedule (every few minutes) " +
      "to see if anything new needs attention.",
    long:
      "Heartbeats keep an agent reactive: a quick scan of the inbox, " +
      "open tasks, and budget alerts. They fire constantly in the " +
      "background, so using a cheaper tier here can dramatically reduce " +
      "your monthly spend without affecting the work that matters.",
    recommendation: "Very frequent → cheaper tier recommended.",
  },
  {
    id: "routine_trigger",
    label: "Routine trigger",
    short:
      "Scheduled jobs you have set up under Routines " +
      `(e.g. "summarise PRs every Monday morning").`,
    long:
      "Routine triggers fire on a cron schedule that you control. The " +
      "workload is predictable and rarely time-critical, so a cheaper " +
      "tier is usually fine. Override per-agent if a specific routine " +
      "needs the smarter model.",
    recommendation: "Predictable workload → cheaper tier usually fine.",
  },
  {
    id: "budget_alert",
    label: "Budget alert",
    short:
      "Agent is notified when a workspace or project budget is near its " +
      "limit and may need to throttle or escalate.",
    long:
      "Budget alerts are quick acknowledgements — the agent reads the " +
      "alert, decides whether to pause work or notify a human, and goes " +
      "back to sleep. There is no heavy reasoning involved, so the " +
      "cheapest tier is almost always enough.",
    recommendation: "Quick acknowledgement work → cheaper tier is enough.",
  },
];

// USE_AGENT_TIER is the sentinel value used in the wake-reason tier
// dropdown to mean "no policy — use whatever tier this agent normally
// uses." Persisting this clears the key from the TierPerReason map.
export const USE_AGENT_TIER = "__inherit__" as const;
