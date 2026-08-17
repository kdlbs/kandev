# Playwright parity fixture

`report.jsonl` is a real Playwright blob report, and `playwright-stats.json` is
the `stats` block of the merged JSON report Playwright produced from that exact
blob. They were recorded together from a seven-test suite covering every outcome
that a naive "passed after a retry" flake count gets wrong:

| Test                           | Attempts                  | Playwright outcome |
| ------------------------------ | ------------------------- | ------------------ |
| always passes                  | passed                    | expected           |
| flakes once then passes        | failed, passed            | flaky              |
| flakes twice then passes       | failed, failed, passed    | flaky              |
| always fails                   | failed x3                 | unexpected         |
| is skipped                     | skipped                   | skipped            |
| is expected to fail and does   | failed (`test.fail()`)    | expected           |
| is expected to fail but passes | passed x3 (`test.fail()`) | unexpected         |

`retry-summary.test.ts` parses the blob and asserts our per-outcome totals equal
`playwright-stats.json`. Regenerate both together, or not at all: the point of
the fixture is that the two sides come from one run.

## Regenerating

The blob and the stats must come from one Playwright run. In a scratch
directory outside the suite, write this config and spec, then run the two
commands below and copy the results here (rewriting absolute paths).

```ts
// playwright.config.ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  retries: 2,
  workers: 1,
  reporter: [["blob", { outputDir: "./blob-report" }]],
  projects: [{ name: "chromium" }],
});
```

```ts
// tests/outcomes.spec.ts
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "@playwright/test";

const counterDir = process.env.FLAKE_COUNTER_DIR!;

function bump(name: string): number {
  const file = path.join(counterDir, name);
  const value = fs.existsSync(file) ? Number(fs.readFileSync(file, "utf8")) : 0;
  fs.writeFileSync(file, String(value + 1));
  return value;
}

test("always passes", () => {
  expect(1).toBe(1);
});

test("flakes once then passes", () => {
  expect(bump("flaky-once")).toBeGreaterThan(0);
});

test("flakes twice then passes", () => {
  expect(bump("flaky-twice")).toBeGreaterThan(1);
});

test("always fails", () => {
  expect(1).toBe(2);
});

test("is skipped", () => {
  test.skip();
});

test("is expected to fail and does", () => {
  test.fail();
  expect(1).toBe(2);
});

test("is expected to fail but passes", () => {
  test.fail();
  expect(1).toBe(1);
});
```

```bash
mkdir -p counters
FLAKE_COUNTER_DIR="$PWD/counters" npx playwright test -c playwright.config.ts
PLAYWRIGHT_JSON_OUTPUT_FILE=merged-report.json \
  npx playwright merge-reports --config playwright.config.ts --reporter=json ./blob-report
```

`merge-reports` unpacks `report.jsonl` next to the `report.zip` it reads; that
unpacked `report.jsonl` is what gets copied here (with `startTime`/`duration`
dropped from the stats block, since they are not part of the contract).
