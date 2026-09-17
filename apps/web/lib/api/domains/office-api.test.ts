import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config so URL assertions are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { listRoutineTriggers, createRoutineTrigger } from "./office-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall() {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return call;
}

const ROUTINE_ID = "routine-1";
const TRIGGERS_URL = `http://api.test/api/v1/office/routines/${ROUTINE_ID}/triggers`;
const NEXT_RUN_AT = "2026-05-05T00:00:00Z";

describe("listRoutineTriggers (AC-001.9)", () => {
  it("requests the routine's triggers and normalizes a snake_case list response", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        triggers: [
          {
            id: "trigger-1",
            routine_id: ROUTINE_ID,
            kind: "cron",
            cron_expression: "*/5 * * * *",
            timezone: "UTC",
            next_run_at: NEXT_RUN_AT,
            last_fired_at: null,
            enabled: true,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-02T00:00:00Z",
          },
        ],
      }),
    );

    const result = await listRoutineTriggers(ROUTINE_ID);

    expect(lastCall()[0]).toBe(TRIGGERS_URL);
    expect(result.triggers).toEqual([
      {
        id: "trigger-1",
        routineId: ROUTINE_ID,
        kind: "cron",
        cronExpression: "*/5 * * * *",
        timezone: "UTC",
        publicId: "",
        signingMode: "",
        nextRunAt: NEXT_RUN_AT,
        lastFiredAt: undefined,
        enabled: true,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-02T00:00:00Z",
      },
    ]);
  });

  it("returns an empty list rather than throwing when the response body is malformed", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ triggers: null }));

    const result = await listRoutineTriggers(ROUTINE_ID);

    expect(result.triggers).toEqual([]);
  });
});

describe("createRoutineTrigger (AC-001.10, AC-002.1)", () => {
  it("POSTs the serialized snake_case body and normalizes the created trigger", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          trigger: {
            id: "trigger-1",
            routine_id: ROUTINE_ID,
            kind: "cron",
            cron_expression: "0 9 * * *",
            timezone: "UTC",
            next_run_at: NEXT_RUN_AT,
            enabled: true,
          },
        },
        201,
      ),
    );

    const result = await createRoutineTrigger(ROUTINE_ID, {
      kind: "cron",
      cronExpression: "0 9 * * *",
      timezone: "UTC",
    });

    const [url, init] = lastCall();
    expect(url).toBe(TRIGGERS_URL);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      kind: "cron",
      cron_expression: "0 9 * * *",
      timezone: "UTC",
    });
    expect(result.trigger).toMatchObject({
      id: "trigger-1",
      cronExpression: "0 9 * * *",
      nextRunAt: NEXT_RUN_AT,
    });
  });

  it("returns a null trigger, not a thrown error, when the response body has no usable trigger", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ trigger: {} }, 201));

    const result = await createRoutineTrigger(ROUTINE_ID, { kind: "cron" });

    expect(result.trigger).toBeNull();
  });

  it("returns a null trigger when the response is missing a trigger object entirely", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}, 201));

    const result = await createRoutineTrigger(ROUTINE_ID, { kind: "cron" });

    expect(result.trigger).toBeNull();
  });
});
