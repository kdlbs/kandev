/**
 * The order agents are listed in, wherever they are listed.
 *
 * The backend already ranks every agent it knows about: each implementation
 * declares a `DisplayOrder`, and `GET /agents/discovery` returns them sorted by
 * it — Claude first, then Codex, with the dev-only mock agent last at 99. That
 * ranking covers agents the scan did *not* find as well as the ones it did, so
 * it is a complete order and not just an order over what happens to be
 * installed on this machine.
 *
 * So neither surface invents an order: both rank saved agents by where the
 * backend put them. The menu and the Agents page listing the same agents in
 * different orders is what this module exists to prevent.
 */

export type DiscoveredAgent = { name: string; available: boolean };
export type SavedAgent = { name: string; profiles: ReadonlyArray<unknown> };

/** The built-in virtual family has no host installation to appear in discovery. */
export const DYNAMIC_AGENT_NAME = "dynamic";

/** Agents the scan currently detects, in rank order. */
export function detectedAgents<D extends DiscoveredAgent>(discovery: ReadonlyArray<D>): D[] {
  return discovery.filter((agent) => agent.available);
}

/**
 * Configured agents the scan no longer detects, in rank order. Empty concrete
 * agent families have nothing to preserve, but the virtual Dynamic family is
 * always retained so settings can create its first profile.
 *
 * The Agents page still renders these — via a synthetic discovery record — so a
 * CLI going missing never hides the profiles configured against it.
 */
export function orphanedAgents<S extends SavedAgent>(
  detected: ReadonlyArray<DiscoveredAgent>,
  saved: ReadonlyArray<S>,
): S[] {
  const detectedNames = new Set(detected.map((agent) => agent.name));
  return saved.filter(
    (agent) =>
      (agent.profiles.length > 0 || agent.name === DYNAMIC_AGENT_NAME) &&
      !detectedNames.has(agent.name),
  );
}

/**
 * `saved`, reordered by the backend's ranking.
 *
 * Ranks against the *whole* discovery list rather than the detected subset, so
 * an agent whose CLI is missing keeps its place among the rest instead of being
 * flung to the end — and so the dev-only mock agent, ranked last by the backend,
 * lands last rather than in the middle of the agents you actually use.
 *
 * Agents the backend does not rank at all keep their saved order, after the
 * ranked ones. A reorder never drops or duplicates an entry.
 */
export function orderAgentsForDisplay<S extends SavedAgent>(
  discovery: ReadonlyArray<DiscoveredAgent>,
  saved: ReadonlyArray<S>,
): S[] {
  const rank = new Map<string, number>();
  discovery.forEach((agent, index) => rank.set(agent.name, index));
  const unranked = rank.size;
  return [...saved].sort(
    (a, b) =>
      (rank.get(a.name) ?? unranked + saved.indexOf(a)) -
      (rank.get(b.name) ?? unranked + saved.indexOf(b)),
  );
}
