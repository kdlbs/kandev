import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

import { e2eSleepPlugin, SLEEP_EXEMPT_FILES } from "./eslint-rules/no-unsanctioned-sleep.mjs";

/**
 * Standalone config for the sleep ratchet and its preview command.
 *
 * This isolates the sleep rule from every other rule in the repo, so a finding
 * here is unambiguously a sleep. Two consumers:
 *
 *   - `scripts/check-new-e2e-sleeps.mjs`, the ratchet, which filters findings
 *     down to the lines a change added.
 *   - `pnpm run lint:e2e-sleeps <path>`, the preview — now mainly a way to
 *     confirm a path is at zero before widening a guard, since the main config
 *     covers all of `e2e/` after the graduation.
 *
 * It stays separate from `eslint.config.mjs` rather than being folded into it
 * because the ratchet needs to lint paths the main config's `ignores` may carve
 * out, and needs the result to contain nothing it has to filter by rule id.
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
    // `@typescript-eslint` is registered for its RULE DEFINITIONS only — none are
    // switched on below. Specs carry inline `eslint-disable` comments naming its
    // rules, and ESLint 9 reports a disable directive for an unknown rule as an
    // error ("Definition for rule … was not found"). Without the plugin the
    // preview exited 1 on nine files that contain no sleep at all, which trains
    // whoever runs it to read a red exit as noise — the habit that lets a real
    // finding through. The ratchet itself filtered those out by rule id, so this
    // never affected the gate's verdict.
    plugins: { "e2e-sleeps": e2eSleepPlugin, "@typescript-eslint": tseslint.plugin },
    rules: { "e2e-sleeps/no-unsanctioned-sleep": "error" },
  },
]);
