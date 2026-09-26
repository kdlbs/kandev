import { RuleTester } from "eslint";
import tseslint from "typescript-eslint";
import { describe, expect, it } from "vitest";

import { noUnsanctionedSleep, SLEEP_EXEMPT_FILES } from "./no-unsanctioned-sleep.mjs";

/**
 * Every `invalid` case below is a shape that must FAIL, and every `valid` case
 * is a shape from this suite that must PASS. The valid list is the load-bearing
 * half: the rule's whole design problem is that `e2e/` holds 701
 * `test.setTimeout()` calls and a handful of race guards that look almost
 * exactly like the ~38 sleeps it must catch.
 *
 * Each valid case names the real call site it stands in for, so a future
 * loosening of the matcher fails here with the reason attached rather than
 * silently flagging correct code.
 */
/**
 * Hand vitest's `describe`/`it` to RuleTester explicitly.
 *
 * Without this, RuleTester finds no test globals (this project does not enable
 * `globals: true`) and falls back to running every case *synchronously during
 * collection*. The cases do execute, but vitest reports `Tests 1 passed (1)` —
 * only the explicit `it` below — so "25 cases passed" and "0 cases ran" produce
 * an identical, green-looking summary. Wiring the hooks makes each case a real
 * test, which is what lets `EXPECTED_CASES` below be an assertion rather than a
 * comment.
 */
RuleTester.describe = describe;
RuleTester.it = it;

const ruleTester = new RuleTester({
  languageOptions: {
    parser: tseslint.parser as never,
    parserOptions: { ecmaVersion: 2022, sourceType: "module" },
  },
});

ruleTester.run("no-unsanctioned-sleep", noUnsanctionedSleep, {
  valid: [
    // --- The 701-site case. A regex ban on `setTimeout` dies here. ---
    { code: "test.setTimeout(60_000);" },
    { code: "e2eTest.setTimeout(120_000);" },

    // --- The sanctioned forms, which are only sanctioned when actually imported. ---
    {
      code: `import { dwell } from "../../helpers/causal-waits";
        await dwell(page, 300, "library-timer", "Radix open delay publishes nothing");`,
    },
    {
      code: `import { dwell } from "./causal-waits";
        await dwell(500, "poll-interval", "backend health poll; no page exists yet");`,
    },
    {
      code: `import { injectLatency } from "../helpers/causal-waits";
        await injectLatency(800, "make the in-flight spinner observable");`,
    },
    // Aliased and namespace imports still resolve to the helper.
    {
      code: `import { dwell as sleep, injectLatency } from "../../helpers/causal-waits";
        await injectLatency(10, "x");`,
    },

    // --- Race guard: several resolution paths, timer is a fallback. ---
    // e2e/tests/auth/auth-lifecycle.spec.ts:218
    {
      code: `new Promise<boolean>((resolve) => {
        const socket = new WebSocket(url);
        socket.onopen = () => resolve(false);
        socket.onclose = () => resolve(true);
        setTimeout(() => resolve(false), 3000);
      });`,
    },

    // --- Cancellable timer: the handle is kept, so something clears it. ---
    // e2e/tests/ssh/concurrency.spec.ts:15 — one statement, still not a sleep.
    {
      code: `new Promise<{ kind: "timeout" }>((resolve) => {
        timer = setTimeout(() => resolve({ kind: "timeout" }), timeoutMs);
      });`,
    },
    {
      code: `new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("timeout")), 30_000);
        ws.onmessage = () => { clearTimeout(timer); resolve(); };
      });`,
    },

    // --- Frame yield, not a duration wait. No carve-out: the executor's one
    // --- statement is a requestAnimationFrame call, not a setTimeout one.
    // e2e/tests/layout/toggle-sidebar-shortcut.spec.ts:123
    {
      code: `new Promise<void>((resolve) => {
        requestAnimationFrame(() => setTimeout(resolve, 0));
      });`,
    },

    // --- In-page code: the discriminator is what the promise RESOLVES ON,
    // --- not which context it runs in. A frame is an event; a duration is not.
    //
    // Instrumentation, sampling geometry frame by frame. The promise resolves on
    // the FRAME — the `setTimeout(…, 0)` only hops out of the rAF callback so the
    // measurement reflects painted geometry. Neither `dwell` nor `injectLatency`
    // exists in the browser context, so flagging this would trip the gate with no
    // legal way to comply. Excluded structurally (the executor's one statement is
    // the rAF call), and pinned here so a future tightening of the nesting rule
    // cannot silently break it.
    // e2e/tests/layout/toggle-sidebar-shortcut.spec.ts:123
    {
      code: `await page.evaluate(() =>
        new Promise<void>((resolve) => {
          requestAnimationFrame(() => setTimeout(resolve, 0));
        }),
      );`,
    },

    // --- A promise that is not a sleep at all. ---
    { code: "new Promise((resolve) => { socket.onopen = () => resolve(); });" },
    { code: "new Promise((resolve) => setTimeout(unrelatedCallback, 100));" },
    // Not the global Promise constructor shape the rule keys on.
    { code: "new Deferred((resolve) => setTimeout(resolve, 100));" },
  ],

  invalid: [
    // --- waitForTimeout, in every receiver position Playwright offers. ---
    {
      code: "await page.waitForTimeout(500);",
      errors: [{ messageId: "waitForTimeout" }],
    },
    {
      code: "await locator.page().waitForTimeout(1000);",
      errors: [{ messageId: "waitForTimeout" }],
    },
    {
      code: 'await page["waitForTimeout"](500);',
      errors: [{ messageId: "waitForTimeout" }],
    },
    {
      code: "await frame.waitForTimeout(HEALTH_POLL_MS);",
      errors: [{ messageId: "waitForTimeout" }],
    },

    // --- Promise sleeps, every form the suite actually contains. ---
    // The 26-site shape.
    {
      code: "await new Promise((r) => setTimeout(r, 500));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // The 6-site shape.
    {
      code: "await new Promise((resolve) => setTimeout(resolve, 1000));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // The typed shape.
    {
      code: "await new Promise<void>((resolve) => setTimeout(resolve, 250));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // Block-bodied, still exactly one statement.
    {
      code: "await new Promise((resolve) => { setTimeout(resolve, 300); });",
      errors: [{ messageId: "promiseSleep" }],
    },
    // A wrapping arrow that only calls resolve is still a sleep.
    {
      code: "await new Promise((resolve) => setTimeout(() => resolve(), 300));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // Non-literal delays are sleeps too.
    {
      code: "await new Promise((r) => setTimeout(r, HEALTH_POLL_MS));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // `window.`/`globalThis.` prefix must not launder it.
    {
      code: "await new Promise((r) => window.setTimeout(r, 100));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // A function expression executor, not just an arrow.
    {
      code: "await new Promise(function (resolve) { setTimeout(resolve, 100); });",
      errors: [{ messageId: "promiseSleep" }],
    },
    // Both bans can fire in one file.
    {
      code: "await page.waitForTimeout(100); await new Promise((r) => setTimeout(r, 100));",
      errors: [{ messageId: "waitForTimeout" }, { messageId: "promiseSleep" }],
    },
    // A sanctioned wait that is not actually bound to the helper. Chunk 1 hit
    // this for real: a patch anchored on an import line whose actual form was
    // `{ type Page, expect }`, so the edit missed and the call site was left
    // referencing an undefined identifier. `no-undef` is off repo-wide and
    // nothing typechecks `e2e/`, so lint passed clean and it would have surfaced
    // as `dwell is not defined` inside a retry path that only runs on slow
    // shards. The token being allowed is not the same as the helper being there.
    {
      code: 'await dwell(page, 500, "unverified", "import silently did not apply");',
      errors: [{ messageId: "unboundWait" }],
    },
    {
      code: 'await injectLatency(200, "import silently did not apply");',
      errors: [{ messageId: "unboundWait" }],
    },
    // Imported, but not from causal-waits: also not the sanctioned helper.
    {
      code: `import { dwell } from "./local-utils";
        await dwell(500, "unverified", "wrong module");`,
      errors: [{ messageId: "unboundWait" }],
    },
    // A locally defined look-alike is not the helper either.
    {
      code: `function dwell(ms) { return ms; }
        await dwell(500, "unverified", "shadowed by a local");`,
      errors: [{ messageId: "unboundWait" }],
    },

    // A DURATION wait is still a duration wait inside `page.evaluate`.
    // `await page.evaluate(() => new Promise((r) => setTimeout(r, 500)))` blocks
    // the test for 500ms exactly like `page.waitForTimeout(500)` — the browser
    // context is a wrapper, not a different kind of wait. Exempting in-page code
    // wholesale would make that one line a laundering wrapper for every sleep in
    // the suite. There IS a legal path: hoist it to `dwell(page, ms, …)` and
    // evaluate afterwards, or resolve on a frame/event as the rAF case above does.
    {
      code: "await page.evaluate(() => new Promise((r) => setTimeout(r, 500)));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // A bound reference to `resolve` is still `resolve`. Flagged so the form
    // cannot be used to launder a sleep past the rule (review finding).
    {
      code: "await new Promise((resolve) => setTimeout(resolve.bind(null), 100));",
      errors: [{ messageId: "promiseSleep" }],
    },
    {
      code: "await new Promise((resolve) => setTimeout(resolve.bind(null).bind(null), 100));",
      errors: [{ messageId: "promiseSleep" }],
    },
    // The finding must land on the METHOD token, not the receiver. On a
    // wrapped call those are different LINES, and the ratchet keeps only
    // findings on lines the change added — so reporting the whole call would
    // attribute this to line 1 and silently drop it (review finding).
    {
      code: "await page\n  .waitForTimeout(500);",
      errors: [{ messageId: "waitForTimeout", line: 2 }],
    },
  ],
});

describe("no-unsanctioned-sleep exemptions", () => {
  /**
   * The exemption list is consumed by two separate eslint configs. Asserting the
   * paths exist stops a rename from silently un-exempting the helper that
   * implements both banned forms — which would surface as the sanctioned wait
   * failing the rule that exists to promote it.
   */
  it("exempts exactly the causal-waits implementation and its own test", () => {
    expect(SLEEP_EXEMPT_FILES).toEqual([
      "e2e/helpers/causal-waits.ts",
      "e2e/helpers/causal-waits.test.ts",
    ]);
  });
});
