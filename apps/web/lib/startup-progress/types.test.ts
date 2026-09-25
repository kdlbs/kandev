import { describe, expect, it } from "vitest";
import { parseStartupSnapshot } from "./types";

const validSnapshot = {
  phase: "applying_migrations",
  boot: 1,
  seq: 2,
  elapsed_ms: 1000,
  phase_elapsed_ms: 500,
  step: {
    id: "stores.services",
    label_key: "startup.step.stores_services",
    measure: "counted",
    unit: "messages",
    elapsed_ms: 500,
    done: 4,
    total: 8,
    rate_per_second: 2,
    eta_ms: 2000,
    since_advance_ms: 100,
    stalled: false,
  },
};

describe("parseStartupSnapshot", () => {
  it("accepts a complete startup snapshot", () => {
    expect(parseStartupSnapshot(validSnapshot)).toEqual(validSnapshot);
  });

  it.each([
    { name: "unknown phase", value: { ...validSnapshot, phase: "booting" } },
    {
      name: "missing step label",
      value: { ...validSnapshot, step: { ...validSnapshot.step, label_key: "" } },
    },
    { name: "negative elapsed time", value: { ...validSnapshot, elapsed_ms: -1 } },
    {
      name: "non-boolean stalled flag",
      value: { ...validSnapshot, step: { ...validSnapshot.step, stalled: 0 } },
    },
  ])("rejects $name", ({ value }) => {
    expect(parseStartupSnapshot(value)).toBeNull();
  });
});
