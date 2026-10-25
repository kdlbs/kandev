import { describe, expect, it } from "vitest";
import { withOwnedUpgrade, validateUpgradeInput, UpgradeBackend } from "./upgrade-backend";

describe("baseline-first upgrade ownership", () => {
  it("rejects candidate startup before any baseline seed is sealed", async () => {
    const backend = Object.create(UpgradeBackend.prototype) as UpgradeBackend;
    await expect(backend.start("candidate")).rejects.toThrow("before baseline seed");
  });
  it("starts Stable before the callback and stops before disposal on seed failure", async () => {
    const events: string[] = [];
    await expect(
      withOwnedUpgrade(
        {
          startBaseline: async () => {
            events.push("stable");
          },
          stop: async () => {
            events.push("stop");
          },
          dispose: async () => {
            events.push("dispose");
          },
        },
        async () => {
          events.push("seed");
          throw new Error("seed failure");
        },
      ),
    ).rejects.toThrow("seed failure");
    expect(events).toEqual(["stable", "seed", "stop", "dispose"]);
  });
  it("cleans partial baseline startup failures", async () => {
    const events: string[] = [];
    await expect(
      withOwnedUpgrade(
        {
          startBaseline: async () => {
            throw new Error("boot failure");
          },
          stop: async () => {
            events.push("stop");
          },
          dispose: async () => {
            events.push("dispose");
          },
        },
        async () => {
          events.push("candidate");
        },
      ),
    ).rejects.toThrow("boot failure");
    expect(events).toEqual(["stop", "dispose"]);
  });
  it("rejects missing manifest and required PostgreSQL instead of skipping", () => {
    expect(() => validateUpgradeInput(undefined, "sqlite", undefined)).toThrow(/manifest/);
    expect(() => validateUpgradeInput("manifest.json", "mysql", undefined)).toThrow(/engine/);
    expect(() => validateUpgradeInput("manifest.json", "postgres", undefined)).toThrow(
      /PostgreSQL/,
    );
  });
});
