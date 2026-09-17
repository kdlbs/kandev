import { describe, expect, it } from "vitest";
import {
  normalizeRoutineTrigger,
  normalizeRoutineTriggerList,
  serializeRoutineTriggerInput,
} from "./office-routine-normalize";

const CRON_EXPRESSION = "*/5 * * * *";

const WIRE_TRIGGER = {
  id: "trigger-1",
  routine_id: "routine-1",
  kind: "cron",
  cron_expression: CRON_EXPRESSION,
  timezone: "UTC",
  public_id: "",
  signing_mode: "",
  next_run_at: "2026-05-05T00:00:00Z",
  last_fired_at: null,
  enabled: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

describe("normalizeRoutineTrigger", () => {
  it("maps every field the UI reads from the snake_case wire shape (AC-001.1)", () => {
    expect(normalizeRoutineTrigger(WIRE_TRIGGER)).toEqual({
      id: "trigger-1",
      routineId: "routine-1",
      kind: "cron",
      cronExpression: CRON_EXPRESSION,
      timezone: "UTC",
      publicId: "",
      signingMode: "",
      nextRunAt: "2026-05-05T00:00:00Z",
      lastFiredAt: undefined,
      enabled: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-02T00:00:00Z",
    });
  });

  it("prefers a present, non-nullish camelCase key over its snake_case counterpart (AC-001.2)", () => {
    const result = normalizeRoutineTrigger({
      ...WIRE_TRIGGER,
      cronExpression: "0 9 * * *",
      cron_expression: "*/5 * * * *",
    });
    expect(result?.cronExpression).toBe("0 9 * * *");
  });

  it("falls through to snake_case when the camelCase key is null, not just absent (AC-001.2)", () => {
    const result = normalizeRoutineTrigger({
      ...WIRE_TRIGGER,
      cronExpression: null,
      cron_expression: CRON_EXPRESSION,
    });
    expect(result?.cronExpression).toBe(CRON_EXPRESSION);
  });

  it("is idempotent: normalizing its own output returns a deeply equal value (AC-001.3)", () => {
    const once = normalizeRoutineTrigger(WIRE_TRIGGER);
    const twice = normalizeRoutineTrigger(once);
    expect(twice).toEqual(once);
  });

  it("carries a non-empty next_run_at/last_fired_at through unparsed (AC-001.4)", () => {
    const result = normalizeRoutineTrigger({
      ...WIRE_TRIGGER,
      next_run_at: "2026-05-05T00:00:00.123456789Z",
      last_fired_at: "2026-05-04T00:00:00Z",
    });
    expect(result?.nextRunAt).toBe("2026-05-05T00:00:00.123456789Z");
    expect(result?.lastFiredAt).toBe("2026-05-04T00:00:00Z");
  });

  it.each([null, undefined, "", 42, {}])(
    "sets next_run_at/last_fired_at to undefined for wire value %p (AC-001.4)",
    (value) => {
      const result = normalizeRoutineTrigger({
        ...WIRE_TRIGGER,
        next_run_at: value,
        last_fired_at: value,
      });
      expect(result?.nextRunAt).toBeUndefined();
      expect(result?.lastFiredAt).toBeUndefined();
    },
  );

  it("keeps a supplied empty string distinct from undefined for other string fields (AC-001.5)", () => {
    const result = normalizeRoutineTrigger({ ...WIRE_TRIGGER, timezone: "" });
    expect(result?.timezone).toBe("");
    expect(typeof result?.timezone).toBe("string");
  });

  it.each([undefined, null, false, 1])(
    "defaults enabled to false for wire value %p (AC-001.6)",
    (value) => {
      expect(normalizeRoutineTrigger({ ...WIRE_TRIGGER, enabled: value })?.enabled).toBe(false);
    },
  );

  it("keeps enabled true only when the wire value is the JSON boolean true (AC-001.6)", () => {
    expect(normalizeRoutineTrigger({ ...WIRE_TRIGGER, enabled: true })?.enabled).toBe(true);
  });
});

describe("normalizeRoutineTrigger field coercion and edge cases", () => {
  it("never carries a secret through, whatever the wire value (AC-001.7)", () => {
    const result = normalizeRoutineTrigger({ ...WIRE_TRIGGER, secret: "shh" }) as unknown as Record<
      string,
      unknown
    >;
    expect(result.secret).toBeUndefined();
    expect(Object.keys(result)).not.toContain("secret");
  });

  it("carries an unrecognized kind through unchanged rather than dropping or coercing it (AC-001.11)", () => {
    expect(normalizeRoutineTrigger({ ...WIRE_TRIGGER, kind: "manual" })?.kind).toBe("manual");
  });

  it("coerces a missing/null/non-string optional field to the empty string, not undefined (AC-001.12)", () => {
    const result = normalizeRoutineTrigger({
      ...WIRE_TRIGGER,
      timezone: null,
      public_id: undefined,
      signing_mode: 7,
    });
    expect(result?.timezone).toBe("");
    expect(result?.publicId).toBe("");
    expect(result?.signingMode).toBe("");
  });

  it("coerces a missing/null/non-string required string field to the empty string (AC-001.12)", () => {
    const result = normalizeRoutineTrigger({ ...WIRE_TRIGGER, routine_id: null, created_at: 5 });
    expect(result?.routineId).toBe("");
    expect(result?.createdAt).toBe("");
  });

  it("drops a trigger whose normalized id is the empty string (AC-001.13)", () => {
    expect(normalizeRoutineTrigger({ ...WIRE_TRIGGER, id: "" })).toBeNull();
    expect(normalizeRoutineTrigger({ ...WIRE_TRIGGER, id: null })).toBeNull();
  });

  it("treats a non-object input as an empty record rather than throwing", () => {
    expect(normalizeRoutineTrigger(null)).toBeNull();
    expect(normalizeRoutineTrigger("nope")).toBeNull();
  });
});

describe("normalizeRoutineTriggerList", () => {
  it("normalizes every array element that is a JSON object, preserving order (AC-001.8)", () => {
    const result = normalizeRoutineTriggerList([
      WIRE_TRIGGER,
      { ...WIRE_TRIGGER, id: "trigger-2" },
    ]);
    expect(result.map((t) => t.id)).toEqual(["trigger-1", "trigger-2"]);
  });

  it("drops array elements that are not JSON objects (AC-001.8)", () => {
    const result = normalizeRoutineTriggerList([WIRE_TRIGGER, "not-an-object", null, 5]);
    expect(result).toHaveLength(1);
  });

  it("drops an element that normalizes to a trigger AC-001.13 rejects (empty id)", () => {
    const result = normalizeRoutineTriggerList([WIRE_TRIGGER, { ...WIRE_TRIGGER, id: "" }]);
    expect(result).toHaveLength(1);
  });

  it("produces an empty array when the input is absent or not an array (AC-001.8)", () => {
    expect(normalizeRoutineTriggerList(undefined)).toEqual([]);
    expect(normalizeRoutineTriggerList(null)).toEqual([]);
    expect(normalizeRoutineTriggerList({})).toEqual([]);
  });
});

describe("serializeRoutineTriggerInput", () => {
  it("emits the supplied fields under their snake_case wire keys (AC-002.1, AC-002.4)", () => {
    expect(
      serializeRoutineTriggerInput({
        kind: "cron",
        cronExpression: "0 9 * * *",
        timezone: "UTC",
        publicId: "pub-1",
        signingMode: "hmac_sha256",
        secret: "shh",
      }),
    ).toEqual({
      kind: "cron",
      cron_expression: "0 9 * * *",
      timezone: "UTC",
      public_id: "pub-1",
      signing_mode: "hmac_sha256",
      secret: "shh",
    });
  });

  it("never emits a camelCase key alongside its snake_case counterpart (AC-002.2)", () => {
    const body = serializeRoutineTriggerInput({ kind: "cron", cronExpression: "0 9 * * *" });
    expect(body).not.toHaveProperty("cronExpression");
    expect(body).toHaveProperty("cron_expression", "0 9 * * *");
  });

  it("omits an unsupplied field's key rather than emitting an empty value (AC-002.3)", () => {
    const body = serializeRoutineTriggerInput({ kind: "webhook" });
    expect(body).toEqual({ kind: "webhook" });
    expect(body).not.toHaveProperty("cron_expression");
  });

  it('emits a supplied empty string as "", not omitted (AC-002.3)', () => {
    const body = serializeRoutineTriggerInput({ kind: "cron", cronExpression: "" });
    expect(body).toHaveProperty("cron_expression", "");
  });
});
