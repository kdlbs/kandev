import { registerCustomTheme } from "@pierre/diffs";
import { bundledThemes } from "shiki";
import type { WorkerPoolManager } from "@pierre/diffs/worker";
import type { EffectiveTheme } from "./theme-types";
import { textmateThemeData } from "./theme-adapters";
import { PIERRE_THEME } from "./colors";

const registered = new Set<string>();
export function registerPierreTheme(theme: EffectiveTheme): string | undefined {
  if (!theme.definition) return undefined;
  const name = `kandev-${theme.digest}`;
  if (!registered.has(name)) {
    registered.add(name);
    registerCustomTheme(name, async () => {
      const base = await bundledThemes[theme.base === "dark" ? "dark-plus" : "light-plus"]();
      return textmateThemeData(theme, base.default);
    });
  }
  return name;
}
type Pool = Pick<WorkerPoolManager, "setRenderOptions" | "getDiffRenderOptions">;
const updates = new WeakMap<Pool, { revision: number; pending: Promise<void> }>();
export function updatePierreTheme(pool: Pool, theme: EffectiveTheme): Promise<void> {
  let state = updates.get(pool);
  if (!state) {
    state = { revision: 0, pending: Promise.resolve() };
    updates.set(pool, state);
  }
  const revision = ++state.revision;
  const current = state;
  const name = registerPierreTheme(theme);
  current.pending = current.pending
    .catch(() => {})
    .then(async () => {
      if (current.revision !== revision) return;
      await pool.setRenderOptions({ ...pool.getDiffRenderOptions(), theme: name ?? PIERRE_THEME });
    });
  return current.pending;
}
