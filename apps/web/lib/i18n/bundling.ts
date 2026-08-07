/**
 * Build-time decision: does this bundle compile in the `pseudo` QA catalog?
 *
 * `pseudo` is the largest catalog we ship (accented characters are multi-byte)
 * and no production user can ever select it — `selectableLocales` has always
 * filtered it out of the switcher. Until this switch existed it was still
 * *bundled*, so every user downloaded it. Filtering at runtime cannot fix that;
 * the entries have to be dropped before rolldown builds the module graph.
 *
 * `import.meta.env.PROD` is the wrong signal, and the reason is the whole
 * difficulty of this change: the e2e harness serves a **production** bundle, so
 * that constant is true there too. e2e is exactly where `pseudo` must survive —
 * `e2e/tests/i18n/pseudo-coverage.spec.ts` is the only oracle that sees copy the
 * jsx-only eslint guard structurally cannot (strings in variables, default
 * parameters, `.ts` helpers, SCREAMING_CASE config tables). So the decision is
 * taken from the *build invocation* instead: `vite dev` and vitest always keep
 * it, and a `vite build` keeps it only when asked to.
 *
 * Kept as a pure function, separate from `vite.config.ts`, so both directions
 * are unit-tested. A bundling switch is only ever exercised by a real
 * production build, and one that is silently stuck on "include" is
 * indistinguishable from one that works.
 */

/** Environment variable that opts a `vite build` into shipping `pseudo`. */
export const PSEUDO_BUNDLE_ENV_VAR = "KANDEV_I18N_BUNDLE_PSEUDO";

/** The same truthy set the Makefile uses (`$(filter 1 true yes,...)`). */
const TRUTHY = new Set(["1", "true", "yes"]);

export function shouldBundlePseudoLocale(options: {
  /** Vite's `ConfigEnv.command`. */
  command: string;
  /** Usually `process.env`. */
  env?: Record<string, string | undefined>;
}): boolean {
  // `serve` covers `pnpm dev` and vitest, which resolves config in serve mode.
  // Neither produces an artifact a user downloads, and both want the oracle.
  if (options.command === "serve") return true;
  const raw = options.env?.[PSEUDO_BUNDLE_ENV_VAR];
  return TRUTHY.has((raw ?? "").trim().toLowerCase());
}
