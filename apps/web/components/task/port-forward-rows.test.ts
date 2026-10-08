import { describe, expect, it } from "vitest";
import { buildPortForwardRows } from "./port-forward-rows";

// @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1, .2, .5
describe("port forwarding projection", () => {
  it("unifies targets once with detection metadata and active targets first in numeric order", () => {
    const detected = [
      { port: 80, address: "*" },
      { port: 9000, address: "127.0.0.1", process: "node" },
      { port: 9000, address: "::1", process: "duplicate" },
    ];
    const manual = [9000, 4000, 4000, 1000];
    const tunnels = new Map([
      [9500, 49500],
      [9000, 49000],
    ]);
    const result = buildPortForwardRows(detected, manual, tunnels);
    expect(result).toEqual([
      { port: 9000, address: "127.0.0.1", process: "node", badge: "detected", tunnelPort: 49000 },
      { port: 9500, badge: "manual", tunnelPort: 49500 },
      { port: 80, address: "*", badge: "detected" },
      { port: 1000, badge: "manual" },
      { port: 4000, badge: "manual" },
    ]);
    expect(detected.map(({ port }) => port)).toEqual([80, 9000, 9000]);
    expect(manual).toEqual([9000, 4000, 4000, 1000]);
    expect([...tunnels]).toEqual([
      [9500, 49500],
      [9000, 49000],
    ]);
  });

  it("has no active targets for proxy-only or empty inputs", () => {
    expect(buildPortForwardRows([], [], new Map())).toEqual([]);
    expect(buildPortForwardRows([], [3000], new Map())).toEqual([{ port: 3000, badge: "manual" }]);
  });
});
