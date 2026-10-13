import { describe, expect, it } from "vitest";
import { backendPorts } from "./backend-ports";

describe("backend fixture port ownership", () => {
  it.each([-1, 30, 0.5, Number.NaN])("rejects an invalid shard offset %s", (offset) => {
    expect(() => backendPorts(offset, { workerIndex: 0, parallelIndex: 0 })).toThrow(/port offset/);
  });

  it.each([-1, 5, 0.5, Number.NaN])("rejects an invalid parallel slot %s", (parallelIndex) => {
    expect(() => backendPorts(0, { workerIndex: 0, parallelIndex })).toThrow(/parallel slot/);
  });

  it("keeps a replacement worker in its original slot across retries and projects", () => {
    const initial = backendPorts(0, { workerIndex: 0, parallelIndex: 0 });
    const replacement = backendPorts(0, { workerIndex: 31, parallelIndex: 0 });
    expect(replacement).toEqual(initial);
  });

  it("does not collide with a live sibling shard when a worker is replaced", () => {
    const replacement = backendPorts(0, { workerIndex: 1, parallelIndex: 0 });
    const sibling = backendPorts(1, { workerIndex: 0, parallelIndex: 0 });
    expect(replacement.backendPort).not.toBe(sibling.backendPort);
    expect(replacement.standaloneAgentctlPort).not.toBe(sibling.standaloneAgentctlPort);
    expect(replacement.agentctlPortMax).toBeLessThan(sibling.agentctlPortBase);
  });

  it("keeps every supported shard and slot range disjoint and inside TCP port bounds", () => {
    const used = new Set<number>();
    for (let offset = 0; offset < 30; offset++) {
      for (let slot = 0; slot < 5; slot++) {
        const ports = backendPorts(offset, { workerIndex: 91, parallelIndex: slot });
        const allocated = [ports.backendPort, ports.standaloneAgentctlPort];
        for (let port = ports.agentctlPortBase; port <= ports.agentctlPortMax; port++) {
          allocated.push(port);
        }
        for (const port of allocated) {
          expect(port).toBeGreaterThan(1023);
          expect(port).toBeLessThan(65536);
          expect(used.has(port)).toBe(false);
          used.add(port);
        }
      }
    }
  });
});
