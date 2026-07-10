import { describe, expect, it } from "vitest";
import { shouldUseSupervisor } from "./backend";

describe("backend supervisor launch policy", () => {
  it("uses the supervisor by default", () => {
    expect(shouldUseSupervisor({})).toBe(true);
  });

  it("can be disabled with KANDEV_NO_SUPERVISOR", () => {
    expect(shouldUseSupervisor({ KANDEV_NO_SUPERVISOR: "true" })).toBe(false);
    expect(shouldUseSupervisor({ KANDEV_NO_SUPERVISOR: "1" })).toBe(true);
  });
});
