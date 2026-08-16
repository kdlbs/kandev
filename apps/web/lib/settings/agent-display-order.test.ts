import { describe, expect, it } from "vitest";

import { detectedAgents, orderAgentsForDisplay, orphanedAgents } from "./agent-display-order";

const CLAUDE = "claude-acp";
const CODEX = "codex-acp";
const MOCK = "mock-agent";

const DISCOVERY = [
  { name: CLAUDE, available: true },
  { name: CODEX, available: false },
  { name: MOCK, available: true },
  { name: "gemini", available: false },
];

const SAVED = [
  { name: CODEX, profiles: [{ id: "p1" }] },
  { name: CLAUDE, profiles: [{ id: "p2" }] },
  { name: MOCK, profiles: [{ id: "p3" }] },
];

describe("detectedAgents", () => {
  it("keeps only what the scan found, in scan order", () => {
    expect(detectedAgents(DISCOVERY).map((agent) => agent.name)).toEqual([CLAUDE, MOCK]);
  });
});

describe("orphanedAgents", () => {
  it("returns configured agents the scan no longer finds", () => {
    const orphans = orphanedAgents(detectedAgents(DISCOVERY), SAVED);

    expect(orphans.map((agent) => agent.name)).toEqual([CODEX]);
  });

  it("ignores an undetected agent that has nothing configured under it", () => {
    // Nothing to preserve, so it earns no group of its own.
    const saved = [{ name: "gemini", profiles: [] }];

    expect(orphanedAgents(detectedAgents(DISCOVERY), saved)).toEqual([]);
  });

  it("keeps the empty virtual Dynamic family visible for profile creation", () => {
    const saved = [{ name: "dynamic", profiles: [] }];

    expect(orphanedAgents(detectedAgents(DISCOVERY), saved).map((agent) => agent.name)).toEqual([
      "dynamic",
    ]);
  });
});

describe("orderAgentsForDisplay", () => {
  it("ranks by the backend's order, detected or not", () => {
    // DISCOVERY is the backend's ranking: claude, codex, mock. Saved order is
    // codex, claude, mock — what the menu used to render.
    expect(orderAgentsForDisplay(DISCOVERY, SAVED).map((agent) => agent.name)).toEqual([
      CLAUDE,
      CODEX,
      MOCK,
    ]);
  });

  it("keeps an undetected agent in rank rather than flinging it to the end", () => {
    // `codex` is ranked above `mock` and stays there even though the scan does
    // not find it — otherwise the dev-only mock agent outranks a real one.
    const ordered = orderAgentsForDisplay(DISCOVERY, SAVED).map((agent) => agent.name);

    expect(ordered.indexOf(CODEX)).toBeLessThan(ordered.indexOf(MOCK));
  });

  it("keeps the saved order when the scan has not hydrated yet", () => {
    expect(orderAgentsForDisplay([], SAVED).map((agent) => agent.name)).toEqual([
      CODEX,
      CLAUDE,
      MOCK,
    ]);
  });

  it("holds the saved order among agents the scan does not mention", () => {
    const saved = [
      { name: "zeta", profiles: [{ id: "p" }] },
      { name: CLAUDE, profiles: [{ id: "p" }] },
      { name: "alpha", profiles: [{ id: "p" }] },
    ];

    // `zeta` and `alpha` are unknown to the scan: detected first, then those two
    // in the order they were saved — not alphabetised behind the app's back.
    expect(orderAgentsForDisplay(DISCOVERY, saved).map((agent) => agent.name)).toEqual([
      CLAUDE,
      "zeta",
      "alpha",
    ]);
  });

  it("drops nothing and duplicates nothing", () => {
    const ordered = orderAgentsForDisplay(DISCOVERY, SAVED);

    expect(ordered).toHaveLength(SAVED.length);
    expect(new Set(ordered.map((agent) => agent.name)).size).toBe(SAVED.length);
  });
});
