import { SHELL_OUTCOMES_BEHAVIOR, SHELL_OUTCOMES_VERSION } from "../protocol.js";
import { ShellOutcomesBehavior } from "./shell-outcomes.js";
import type { Behavior } from "./types.js";

/**
 * The production behavior list is compiled and static. Adding a behavior is an
 * explicit code change on both the plugin and receiver sides; the loader never
 * accepts a module name from configuration.
 */
export function productionBehaviors(): Behavior[] {
  return [new ShellOutcomesBehavior()];
}

export function behaviorRefs(behaviors: readonly Behavior[]): { id: string; version: number }[] {
  return behaviors.map((behavior) => ({ id: behavior.id, version: behavior.version }));
}

export { SHELL_OUTCOMES_BEHAVIOR, SHELL_OUTCOMES_VERSION };
