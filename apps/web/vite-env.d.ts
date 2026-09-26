/// <reference types="vite/client" />

/**
 * Injected by the `kandev:i18n-pseudo-locale-bundling` plugin in `vite.config.ts`.
 * `false` in a plain `vite build`, `true` under `vite dev`, vitest, and a build
 * run with `KANDEV_I18N_BUNDLE_PSEUDO=1` (the e2e artifact).
 */
declare const __KANDEV_PSEUDO_LOCALE_BUNDLED__: boolean;
