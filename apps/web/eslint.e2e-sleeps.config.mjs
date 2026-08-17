import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

import { e2eSleepPlugin, SLEEP_EXEMPT_FILES } from "./eslint-rules/no-unsanctioned-sleep.mjs";

/**
 * Standalone config for the sleep ratchet and its preview command.
 *
 * The main config (eslint.config.mjs) errors only on `e2eSleepGuardFiles` — the
 * directories the causal-waits conversion has already driven to zero. That is
 * deliberate: `pnpm run lint` is `eslint --max-warnings 0`, so registering this
 * rule repo-wide at *any* severity, warning included, would fail the build on
 * the ~130 sleeps that predate it and break every unrelated PR until the
 * conversion finishes.
 *
 * This config applies the same rule to whatever paths it is given, regardless of
 * that allowlist. Two consumers:
 *
 *   - `scripts/check-new-e2e-sleeps.mjs`, the ratchet, which filters findings
 *     down to the lines a change added.
 *   - `pnpm run lint:e2e-sleeps <path>`, the work-list generator for whoever is
 *     converting a directory next.
 *
 * The rule and the exemption list are imported, not restated, so the preview,
 * the gate and the editor cannot disagree about what counts as a sleep.
 */
export default defineConfig([
  {
    // BOTH extensions. `isE2eCandidate` selects `.tsx` too, so a config matching
    // only `.ts` left a selected file matching no config at all — and
    // `warnIgnored: false` in the ratchet suppressed the one signal that would
    // have shown it, so the gate passed silently. Keep this in step with the
    // selector.
    files: ["**/*.ts", "**/*.tsx"],
    ignores: [...SLEEP_EXEMPT_FILES, "node_modules/**", "dist/**"],
    languageOptions: { parser: tseslint.parser },
    plugins: { "e2e-sleeps": e2eSleepPlugin },
    rules: { "e2e-sleeps/no-unsanctioned-sleep": "error" },
  },
]);
