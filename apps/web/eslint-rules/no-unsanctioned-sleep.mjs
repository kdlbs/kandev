/**
 * `no-unsanctioned-sleep` — an unconditional wall-clock sleep in `e2e/` must go
 * through `dwell()` or `injectLatency()` from `e2e/helpers/causal-waits.ts`.
 *
 * Those two helpers demand a category and a reason. A raw sleep demands nothing,
 * which is why it is indistinguishable — at a glance and to a grep — from
 * somebody who could not find the right event and reached for a number instead.
 * The whole point of the causal-waits work is that the sanctioned forms are
 * greppable and self-documenting; that only holds if the unsanctioned ones
 * cannot be added silently.
 *
 * ## Why this is an AST rule and not a grep
 *
 * `e2e/` contains **701** `test.setTimeout(60_000)` calls — Playwright's
 * per-test timeout setter, which has nothing to do with sleeping and must never
 * be flagged — against ~38 real promise sleeps. Any `setTimeout` pattern that a
 * regex can express either drowns in those 701 or misses the multi-line sleeps.
 * It also cannot separate a sleep from a `Promise.race` timeout guard, which is
 * correct code that looks almost identical. The AST separates all three exactly.
 *
 * ## What is reported
 *
 * 1. **`waitForTimeout`**, in any receiver position. Playwright exposes it on
 *    `Page`, `Frame` and `Locator.page()`, so the receiver is not fixed; the
 *    method name is. `dwell(page, ms, category, reason)` is the legal form.
 *
 * 2. **The promise-sleep idiom** — `new Promise((r) => setTimeout(r, ms))` — in
 *    the narrow shape where the timer is the promise's *only* resolution path.
 *    `dwell(ms, category, reason)` is the legal form where no `Page` exists.
 *
 * ## What is deliberately NOT reported, and why the narrowing is principled
 *
 * The executor must contain **exactly one statement**, that statement's value
 * must be **discarded**, and its first argument must **resolve the promise**.
 * Each clause excludes a real, correct pattern in this suite:
 *
 * - *More than one statement* excludes a race guard. `auth-lifecycle.spec.ts`
 *   registers `socket.onopen/onclose/onerror` and then a `setTimeout` fallback:
 *   the timer is one of four resolution paths, not a sleep.
 * - *Value discarded* excludes a cancellable timer. `concurrency.spec.ts` has a
 *   lone `timer = setTimeout(() => resolve({kind:"timeout"}), ms)` inside its
 *   executor — one statement, but it keeps the handle because something else
 *   will `clearTimeout` it. **A sleep never keeps the handle**; keeping it is
 *   precisely the intent to cancel, which means another path resolves first.
 * - *First argument resolves the promise* keeps the diagnostic honest: it makes
 *   the finding "this is a sleep" rather than merely "this has the right shape".
 *
 * The same three clauses dissolve the `requestAnimationFrame(() =>
 * setTimeout(resolve, 0))` frame-yield in `toggle-sidebar-shortcut.spec.ts`
 * without a carve-out: the executor's one statement is a `requestAnimationFrame`
 * call, not a `setTimeout` one. That is not a convenient exception — the promise
 * there resolves on the **frame**, and the `setTimeout(…, 0)` only hops out of
 * the rAF callback so the measurement reflects painted geometry. It is not a
 * duration wait, so a rule about duration waits should not see it. A blanket
 * "any `setTimeout(…, 0)` is fine" carve-out would have been a loophole; this
 * needs no carve-out at all.
 *
 * ## Severity
 *
 * An **error**, but only on {@link e2eSleepGuardFiles} — the directories already
 * converted to zero. Not a warning either: `pnpm lint` is
 * `eslint --max-warnings 0`, so a repo-wide registration at *any* severity would
 * fail the build on the ~126 sleeps that predate this rule and block every
 * unrelated PR, which is the mistake the first i18n attempt made.
 *
 * Everywhere else the gate is `scripts/check-new-e2e-sleeps.mjs`, which judges
 * only the lines a change added. Once the conversion reaches zero the guard list
 * collapses to `e2e/**\/*.ts` and the ratchet becomes belt-and-braces.
 */

/**
 * Files exempt from the rule, repo-relative from `apps/web`.
 *
 * `causal-waits.ts` implements `dwell` and `injectLatency` and therefore
 * contains both banned forms by construction — `dwell` delegates to
 * `page.waitForTimeout(ms)` when a page exists and to a promise sleep when one
 * does not. Without this the sanctioned helper is the one file that can never
 * satisfy the rule it exists to serve.
 *
 * Its own `*.test.ts` is exempt for the same reason: tests for a sleep helper
 * necessarily construct sleeps.
 *
 * Exported so `eslint.config.mjs` and `eslint.e2e-sleeps.config.mjs` cannot
 * drift — an exemption that applies in the editor but not in the gate (or the
 * reverse) is a difference nobody would notice until it fired.
 */
export const SLEEP_EXEMPT_FILES = [
  "e2e/helpers/causal-waits.ts",
  "e2e/helpers/causal-waits.test.ts",
];

/**
 * Paths where this rule is an **error** in `eslint.config.mjs`.
 *
 * Same opt-in shape as `i18nGuardFiles`, and for the same reason: `pnpm lint` is
 * `eslint --max-warnings 0`, so a repo-wide registration at any severity fails
 * the build on the ~126 sleeps that predate the rule and blocks every unrelated
 * PR. Listing only directories already at zero means this can only ever tighten.
 *
 * The seed was measured, not guessed — `pnpm run e2e:waits` reports remaining
 * raw sleeps per file, and every directory below had none at the time it was
 * added. Adding a path is the only safe edit: **never remove one to make a build
 * pass.** A removal means somebody added a sleep to a directory that had none,
 * which is exactly the regression this exists to catch.
 *
 * The conversion effort is working through the directories that are *not* here
 * (`tests/{auth,chat,layout,office,session,settings,ssh,system,task,terminal,
 * workflow,…}`); each should be appended as it reaches zero. When the list would
 * cover all of `e2e/`, replace it with a plain `["e2e/**\/*.ts"]` — that is the
 * graduation, and at that point the diff-scanning ratchet becomes a second line
 * of defence rather than the only one.
 */
export const e2eSleepGuardFiles = [
  // Shard planning, flake and timing reports. Tooling rather than specs, and not
  // in any conversion chunk's path.
  "e2e/scripts/**/*.ts",
  // Test directories measured at zero raw sleeps.
  "e2e/tests/cli-mode/**/*.ts",
  "e2e/tests/docker/**/*.ts",
  "e2e/tests/github/**/*.ts",
  "e2e/tests/i18n/**/*.ts",
  "e2e/tests/kanban/**/*.ts",
  "e2e/tests/preview/**/*.ts",
];

const WAIT_FOR_TIMEOUT = "waitForTimeout";
const SET_TIMEOUT = "setTimeout";

/** `setTimeout`, `window.setTimeout`, `globalThis.setTimeout`. */
function isSetTimeoutCallee(node) {
  if (node.type === "Identifier") return node.name === SET_TIMEOUT;
  if (node.type !== "MemberExpression" || node.computed) return false;
  return node.property.type === "Identifier" && node.property.name === SET_TIMEOUT;
}

/** The method name a call invokes, for `a.b()` and `a["b"]()` alike. */
function calleeMethodName(node) {
  if (node.type === "Identifier") return node.name;
  if (node.type !== "MemberExpression") return null;
  if (!node.computed) return node.property.type === "Identifier" ? node.property.name : null;
  return node.property.type === "Literal" && typeof node.property.value === "string"
    ? node.property.value
    : null;
}

/**
 * The executor's own statements, or `null` if it is not a function we can read.
 *
 * An expression-bodied arrow (`(r) => setTimeout(r, 5)`) is normalised to a
 * single-entry list so both forms flow through one check. Nested functions are
 * NOT unwrapped: that is exactly what keeps the rAF frame-yield out.
 */
function executorStatements(executor) {
  if (executor?.type !== "ArrowFunctionExpression" && executor?.type !== "FunctionExpression") {
    return null;
  }
  if (executor.body.type !== "BlockStatement") return [executor.body];
  return executor.body.body.map((statement) =>
    statement.type === "ExpressionStatement" ? statement.expression : statement,
  );
}

/** The single expression a function evaluates to, or `null` if it does more. */
function soleBodyExpression(node) {
  if (node.body.type !== "BlockStatement") return node.body;
  if (node.body.body.length !== 1) return null;
  const [only] = node.body.body;
  return only.type === "ExpressionStatement" ? only.expression : null;
}

/** Whether `node` is a call to `resolve`, or a function whose body only calls it. */
function resolvesPromise(node, resolveName) {
  if (!node) return false;
  if (node.type === "Identifier") return node.name === resolveName;
  if (node.type !== "ArrowFunctionExpression" && node.type !== "FunctionExpression") return false;

  const body = soleBodyExpression(node);
  if (body?.type !== "CallExpression" || body.callee.type !== "Identifier") return false;
  return body.callee.name === resolveName;
}

/**
 * The `setTimeout` call that makes `node` an unconditional sleep, or `null`.
 *
 * Every clause here is load-bearing — see the module comment for the real call
 * site each one protects.
 */
function promiseSleepCall(node) {
  if (node.callee.type !== "Identifier" || node.callee.name !== "Promise") return null;

  const executor = node.arguments[0];
  const resolveParam = executor?.params?.[0];
  if (resolveParam?.type !== "Identifier") return null;

  const statements = executorStatements(executor);
  // Exactly one statement: more than one means the timer is a fallback among
  // several resolution paths (a race guard), not the wait itself.
  if (statements?.length !== 1) return null;

  const [only] = statements;
  // A bare call, so the handle is discarded. `timer = setTimeout(...)` arrives
  // as an AssignmentExpression and stops here: keeping the handle is the intent
  // to cancel, and a sleep is never cancelled.
  if (only.type !== "CallExpression" || !isSetTimeoutCallee(only.callee)) return null;

  return resolvesPromise(only.arguments[0], resolveParam.name) ? only : null;
}

/** @type {import("eslint").Rule.RuleModule} */
export const noUnsanctionedSleep = {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require dwell()/injectLatency() from e2e/helpers/causal-waits.ts instead of raw " +
        "page.waitForTimeout() or hand-rolled promise sleeps.",
    },
    schema: [],
    messages: {
      waitForTimeout:
        "Raw `waitForTimeout()` is not a sanctioned wait. Use `dwell(page, ms, category, reason)` " +
        "from `e2e/helpers/causal-waits.ts`, or better, wait on the cause with `waitForHttp` / " +
        "`watchWs`. See e2e/README.md.",
      promiseSleep:
        "Hand-rolled promise sleep is not a sanctioned wait. Use `dwell(ms, category, reason)` " +
        "from `e2e/helpers/causal-waits.ts`, or `injectLatency(ms, reason)` inside a " +
        "`page.route()` handler. See e2e/README.md.",
    },
  },
  create(context) {
    return {
      CallExpression(node) {
        if (calleeMethodName(node.callee) === WAIT_FOR_TIMEOUT) {
          context.report({ node, messageId: "waitForTimeout" });
        }
      },
      NewExpression(node) {
        const sleep = promiseSleepCall(node);
        if (sleep) context.report({ node: sleep, messageId: "promiseSleep" });
      },
    };
  },
};

/** Flat-config plugin object, so both configs register the rule the same way. */
export const e2eSleepPlugin = {
  rules: { "no-unsanctioned-sleep": noUnsanctionedSleep },
};

export const SLEEP_RULE_ID = "e2e-sleeps/no-unsanctioned-sleep";
