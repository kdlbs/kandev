import type { Ghostty } from 'ghostty-web';

let ghosttyPromise: Promise<Ghostty> | null = null;

/**
 * Load and cache the Ghostty WASM instance.
 * Ensures WASM is only loaded once and reused across all terminal instances.
 */
export async function loadGhostty(): Promise<Ghostty> {
  if (!ghosttyPromise) {
    ghosttyPromise = (async () => {
      const mod = await import('ghostty-web');
      return mod.Ghostty.load('/wasm/ghostty-vt.wasm');
    })();
  }
  return ghosttyPromise;
}
