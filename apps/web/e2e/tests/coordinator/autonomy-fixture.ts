import type { Page } from "@playwright/test";

const HOUR_MS = 3_600_000;
const MINUTE_MS = 60_000;

export type AutonomyRoute = { body: () => Record<string, unknown> };

/** A held-by-containment autonomy read: five pending wakes, a spend pill over its ceiling, and a stop that was not confirmed. */
export function heldAutonomyBody(): Record<string, unknown> {
  const now = Date.now();
  const iso = (offsetMs: number) => new Date(now + offsetMs).toISOString();
  return {
    server_time: iso(0),
    autonomy_enabled: true,
    admission: { ok: false, reason: "containment", detail: "no_kandev_credential" },
    pending_wakes: 5,
    oldest_pending_at: iso(-3 * HOUR_MS),
    last_woke_at: iso(-2 * HOUR_MS),
    last_turn: {
      id: "turn-open",
      coordinator_id: "c",
      conversation_task_id: "t",
      session_id: "s-open",
      started_at: iso(-30 * MINUTE_MS),
      finished_at: null,
      outcome: null,
      wake_count: 2,
      denied_permissions: 0,
      cost_subcents: null,
      stop_requested_at: iso(-10 * MINUTE_MS),
      stop_state: "stop_failing",
    },
    containment: {
      conditions: [
        { name: "executor_isolated", met: true, detail: "docker" },
        { name: "auth_enabled", met: true, detail: "" },
        { name: "no_kandev_credential", met: false, detail: "KANDEV_API_KEY" },
        { name: "no_extra_tools", met: true, detail: "" },
      ],
    },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 104_000,
      mean_daily_subcents_7d: 52_000,
      mean_known: true,
      ceiling_subcents: 100_000,
    },
  };
}

export async function stubAutonomyRead(
  page: Page,
  coordinatorId: string,
  body: () => Record<string, unknown> = heldAutonomyBody,
): Promise<void> {
  await page.route(new RegExp(`/coordinators/${coordinatorId}/autonomy$`), (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body()),
    }),
  );
}

export function runBody(turnId: string, coordinatorId: string): Record<string, unknown> {
  return {
    id: turnId,
    coordinator_id: coordinatorId,
    conversation_task_id: "t",
    session_id: "s",
    started_at: new Date().toISOString(),
    finished_at: null,
    outcome: null,
    wake_count: 3,
    denied_permissions: 1,
    cost_subcents: null,
    stop_requested_at: null,
    stop_state: null,
    wakes: [
      {
        id: "w1",
        kind: "question",
        task_id: "task-1",
        task_identifier: "KAN-418",
        task_title: "Which database should the billing service use for invoices",
      },
      {
        id: "w2",
        kind: "stall",
        task_id: "task-2",
        task_identifier: "KAN-419",
        task_title: "Stalled",
      },
      { id: "w3", kind: "question", task_id: "task-3", task_identifier: null, task_title: null },
    ],
  };
}
