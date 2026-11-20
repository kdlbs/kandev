import { describe, expect, it } from "vitest";
import { blockingCircuitCount, isRoutingCircuits, type RoutingCircuit } from "./routing-circuits";

const circuit = (state: RoutingCircuit["state"], until?: string): RoutingCircuit => ({
  resource_key: state,
  scope: "credential",
  scope_value: "credential-id",
  state,
  failures: 1,
  until,
});

describe("routing circuits", () => {
  it("counts only circuits that currently block dispatch", () => {
    const now = Date.parse("2026-01-01T00:00:00Z");
    expect(
      blockingCircuitCount(
        [
          circuit("closed"),
          circuit("open"),
          circuit("half_open"),
          circuit("open", "2025-12-31T23:59:59Z"),
          circuit("open", "2026-01-01T00:00:00Z"),
          circuit("open", "2026-01-01T00:00:01Z"),
        ],
        now,
      ),
    ).toBe(3);
  });

  it("rejects malformed circuit responses", () => {
    expect(isRoutingCircuits({ circuits: [circuit("open")] })).toBe(true);
    expect(isRoutingCircuits({ circuits: [{ ...circuit("open"), until: "invalid" }] })).toBe(false);
    expect(isRoutingCircuits({ circuits: [{ ...circuit("open"), failures: -1 }] })).toBe(false);
  });
});
