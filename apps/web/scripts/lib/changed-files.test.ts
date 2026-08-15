import { describe, expect, it } from "vitest";

import { isCandidate, isE2eCandidate } from "./changed-files.mjs";

/**
 * `isCandidate` decides which files the **live i18n ratchet** judges, and had no
 * test until the sleep ratchet needed the same git plumbing with a different
 * predicate. These cases pin the existing behaviour so that extraction is
 * provably behaviour-preserving, and pin the new predicate as its exact
 * complement over the `e2e/` tree.
 */
describe("isCandidate (i18n ratchet file selection)", () => {
  it.each([
    "apps/web/components/task/card.tsx",
    "apps/web/app/settings/page.tsx",
    "apps/web/hooks/domains/kanban/use-board.ts",
    "apps/web/lib/state/store.ts",
    "apps/web/src/settings-routes.tsx",
  ])("includes UI source %s", (path) => {
    expect(isCandidate(path)).toBe(true);
  });

  it.each([
    // Tests build fixtures out of literals on purpose.
    ["apps/web/components/task/card.test.tsx", "unit test"],
    ["apps/web/lib/util.spec.ts", "spec file"],
    // Not UI source.
    ["apps/web/e2e/tests/task/chat.spec.ts", "e2e tree"],
    ["apps/web/scripts/check-i18n-keys.mjs", "build script"],
    ["apps/web/eslint.config.mjs", "config, not .ts"],
    ["apps/web/components/task/styles.css", "not TypeScript"],
    ["apps/backend/internal/office/router.go", "outside apps/web"],
    ["apps/packages/ui/src/button.tsx", "outside apps/web"],
  ])("excludes %s (%s)", (path) => {
    expect(isCandidate(path)).toBe(false);
  });
});

describe("isE2eCandidate (sleep ratchet file selection)", () => {
  it.each([
    "apps/web/e2e/tests/task/chat.spec.ts",
    "apps/web/e2e/helpers/api-client.ts",
    "apps/web/e2e/fixtures/backend.ts",
    "apps/web/e2e/pages/task-page.ts",
    // Exemption is the eslint config's job, not the selector's — this path is
    // deliberately still selected here.
    "apps/web/e2e/helpers/causal-waits.ts",
  ])("includes %s", (path) => {
    expect(isE2eCandidate(path)).toBe(true);
  });

  it.each([
    ["apps/web/components/task/card.tsx", "UI source, not e2e"],
    ["apps/web/scripts/check-new-e2e-sleeps.mjs", "build script"],
    ["apps/web/e2e/README.md", "not TypeScript"],
    ["apps/web/e2e/scripts/seed.js", "plain JS is not parsed by the rule config"],
    ["apps/backend/internal/office/router.go", "outside apps/web"],
  ])("excludes %s (%s)", (path) => {
    expect(isE2eCandidate(path)).toBe(false);
  });

  /**
   * The two predicates must not both claim a file: the i18n guard would demand
   * catalog keys for test fixture strings, which is why `e2e/` was excluded from
   * it in the first place.
   */
  it("never overlaps with the i18n predicate", () => {
    const paths = [
      "apps/web/e2e/tests/task/chat.spec.ts",
      "apps/web/e2e/helpers/api-client.ts",
      "apps/web/components/task/card.tsx",
      "apps/web/lib/state/store.ts",
    ];
    for (const path of paths) {
      expect(isCandidate(path) && isE2eCandidate(path)).toBe(false);
    }
  });
});
