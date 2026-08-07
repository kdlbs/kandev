/**
 * The order agents are listed in, wherever they are listed.
 *
 * The Agents page shows them in two groups: the ones the CLI scan currently
 * detects, in scan order, then agents that are configured but whose CLI the scan
 * no longer finds — those keep a group of their own so their profiles never
 * vanish. That grouping is the page's, but it is not the page's alone: the
 * settings menu lists the same agents, and listing them in a different order
 * makes the two read as different sets of agents.
 *
 * So the rule lives here and both read it, rather than the menu re-deriving an
 * order and drifting from the page it opens.
 */

export type DiscoveredAgent = { name: string; available: boolean };
export type SavedAgent = { name: string; profiles: ReadonlyArray<unknown> };

/** Agents the scan currently detects, in scan order. */
export function detectedAgents<D extends DiscoveredAgent>(
  discovery: ReadonlyArray<D>,
): D[] {
  return discovery.filter((agent) => agent.available);
}

/**
 * Configured agents the scan no longer detects, in saved order. Only ones with
 * profiles: an agent with nothing configured under it has nothing to preserve.
 */
export function orphanedAgents<S extends SavedAgent>(
  detected: ReadonlyArray<DiscoveredAgent>,
  saved: ReadonlyArray<S>,
): S[] {
  const detectedNames = new Set(detected.map((agent) => agent.name));
  return saved.filter((agent) => agent.profiles.length > 0 && !detectedNames.has(agent.name));
}

/**
 * `saved`, reordered to match how the Agents page lists them. Agents the scan
 * does not mention at all keep their saved order at the end, so a list is never
 * silently shortened by reordering it.
 */
export function orderAgentsForDisplay<S extends SavedAgent>(
  discovery: ReadonlyArray<DiscoveredAgent>,
  saved: ReadonlyArray<S>,
): S[] {
  const rank = new Map<string, number>();
  detectedAgents(discovery).forEach((agent, index) => rank.set(agent.name, index));
  const detectedCount = rank.size;
  // Stable: equal ranks keep their saved order, which is what the page's second
  // group renders in.
  return [...saved].sort(
    (a, b) =>
      (rank.get(a.name) ?? detectedCount + saved.indexOf(a)) -
      (rank.get(b.name) ?? detectedCount + saved.indexOf(b)),
  );
}
