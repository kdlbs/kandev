import { describe, expect, it } from "vitest";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { collectDescendantIds, reportsToOptions } from "./reports-to-options";

function agent(id: string, reportsTo?: string): AgentProfile {
  return { id, name: id, reportsTo } as AgentProfile;
}

describe("collectDescendantIds", () => {
  it("returns the direct child of the root", () => {
    const agents = [agent("ceo"), agent("manager", "ceo")];
    expect(collectDescendantIds(agents, "ceo")).toEqual(new Set(["manager"]));
  });

  it("returns a grandchild transitively", () => {
    const agents = [agent("ceo"), agent("manager", "ceo"), agent("worker", "manager")];
    expect(collectDescendantIds(agents, "ceo")).toEqual(new Set(["manager", "worker"]));
  });

  it("excludes unrelated agents", () => {
    const agents = [agent("ceo"), agent("manager", "ceo"), agent("other")];
    expect(collectDescendantIds(agents, "ceo")).toEqual(new Set(["manager"]));
  });

  it("does not loop on a chain with a missing parent", () => {
    const agents = [agent("orphan", "does-not-exist")];
    expect(collectDescendantIds(agents, "orphan")).toEqual(new Set());
  });

  it("returns an empty set for a leaf agent", () => {
    const agents = [agent("ceo"), agent("worker", "ceo")];
    expect(collectDescendantIds(agents, "worker")).toEqual(new Set());
  });
});

describe("reportsToOptions", () => {
  it("excludes the agent itself", () => {
    const agents = [agent("ceo"), agent("worker", "ceo")];
    const options = reportsToOptions(agents, "worker");
    expect(options.map((o) => o.id)).not.toContain("worker");
  });

  it("excludes a direct child", () => {
    const agents = [agent("ceo"), agent("manager", "ceo"), agent("worker", "manager")];
    const options = reportsToOptions(agents, "ceo");
    expect(options.map((o) => o.id)).not.toContain("manager");
  });

  it("excludes a grandchild", () => {
    const agents = [agent("ceo"), agent("manager", "ceo"), agent("worker", "manager")];
    const options = reportsToOptions(agents, "ceo");
    expect(options.map((o) => o.id)).not.toContain("worker");
  });

  it("keeps an unrelated agent", () => {
    const agents = [agent("ceo"), agent("manager", "ceo"), agent("other")];
    const options = reportsToOptions(agents, "manager");
    expect(options.map((o) => o.id)).toContain("other");
  });

  it("keeps an unrelated agent when a chain has a missing parent", () => {
    const agents = [agent("orphan", "does-not-exist"), agent("other")];
    const options = reportsToOptions(agents, "orphan");
    expect(options.map((o) => o.id)).toEqual(["other"]);
  });
});
