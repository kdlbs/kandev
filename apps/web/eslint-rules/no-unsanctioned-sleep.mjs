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
 *   "Resolves the promise" includes bound references — `resolve.bind(null)` is
 *   still `resolve`, and recursively so, since a form that laundered a sleep
 *   past the rule would be worth more to a determined author than to an honest
 *   one.
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
 * An **error** across {@link e2eSleepGuardFiles}, which now covers all of
 * `e2e/`. It began as a narrow allowlist of directories already converted to
 * zero — `pnpm lint` is `eslint --max-warnings 0`, so a repo-wide registration
 * at *any* severity would have failed the build on the sleeps that predated the
 * rule and blocked every unrelated PR, the mistake the first i18n attempt made.
 * The conversion has finished, so that cost is gone and the list has graduated.
 *
 * `scripts/check-new-e2e-sleeps.mjs` still judges only the lines a change added.
 * It is the belt to this rule's braces: it covers anything a future `ignores`
 * entry carves out, and it attributes a finding to the change rather than to the
 * file it lives in.
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
 * **Graduated.** This started as an opt-in allowlist of the handful of
 * directories the causal-waits conversion had already driven to zero, for the
 * same reason `i18nGuardFiles` is one: `pnpm lint` is `eslint --max-warnings 0`,
 * so a repo-wide registration at any severity would have failed the build on the
 * ~153 sleeps that predated the rule and blocked every unrelated PR — the
 * mistake the first i18n attempt made.
 *
 * The conversion has since finished. `pnpm run lint:e2e-sleeps e2e` reports zero
 * findings across the whole tree, so the allowlist has collapsed to all of
 * `e2e/`, which is what the list was always going to become. The cost that
 * justified the narrow seed no longer exists: a repo-wide error now fails only
 * a PR that *adds* a sleep, which is the entire point.
 *
 * With the rule covering the tree, `scripts/check-new-e2e-sleeps.mjs` becomes a
 * second line of defence rather than the only one. It is still worth keeping:
 * it is what catches a sleep added to a path a future `ignores` entry carves
 * out, and it reports against the fork point rather than the file, so its
 * message names the line the change added.
 *
 * **Never narrow this back to make a build pass.** A narrowing means somebody
 * added a sleep and deleted the guard rather than the sleep — exactly the
 * regression this exists to catch. `e2e-sleep-wiring.test.ts` asserts coverage
 * behaviourally, through ESLint's own config resolution, so it fails on a
 * narrowing however the glob is rewritten.
 */
export const e2eSleepGuardFiles = ["e2e/**/*.{ts,tsx}"];

/**
 * Directories that were measured at zero and listed individually before the
 * graduation above. Retained as the wiring test's regression corpus: each must
 * still resolve to an error, so collapsing the list cannot silently drop one.
 */
export const e2eSleepSeededGuardDirs = [
  // Shard planning, flake and timing reports. Tooling rather than specs.
  "e2e/scripts",
  "e2e/tests/cli-mode",
  "e2e/tests/docker",
  "e2e/tests/github",
  "e2e/tests/i18n",
  "e2e/tests/kanban",
  "e2e/tests/preview",
];

/**
 * The sanctioned waits. Calling one is only sanctioned if the identifier really
 * IS that helper — see {@link resolveWaitBinding}.
 */
const SANCTIONED_WAITS = new Set(["dwell", "injectLatency"]);

/** `…/causal-waits`, with or without an extension. */
const CAUSAL_WAITS_MODULE = /(^|\/)causal-waits(\.[jt]s)?$/;

/** The nearest binding for `name`, walking out through enclosing scopes. */
function lookupVariable(scope, name) {
  for (let current = scope; current; current = current.upper) {
    const found = current.variables.find((variable) => variable.name === name);
    if (found) return found;
  }
  return null;
}

/**
 * Whether `variable` is really the helper from `causal-waits`.
 *
 * An unresolved identifier is a `ReferenceError` waiting for the machine slow
 * enough to reach it, and a `dwell` imported from somewhere else is not the
 * helper this rule promotes. Both answer `false`.
 */
function isCausalWaitsBinding(variable) {
  if (!variable) return false;
  return variable.defs.some(
    (def) =>
      def.type === "ImportBinding" && CAUSAL_WAITS_MODULE.test(String(def.parent.source.value)),
  );
}

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

/** Whether `node` is `X.bind(...)`, returning `X`; otherwise `null`. */
function boundTarget(node) {
  if (node.type !== "CallExpression") return null;
  const { callee } = node;
  if (callee.type !== "MemberExpression" || callee.computed) return null;
  if (callee.property.type !== "Identifier" || callee.property.name !== "bind") return null;
  return callee.object;
}

/** Whether `node` is a call to `resolve`, or a function whose body only calls it. */
function resolvesPromise(node, resolveName) {
  if (!node) return false;
  if (node.type === "Identifier") return node.name === resolveName;
  // A bound reference to `resolve` is still `resolve`:
  // `setTimeout(resolve.bind(null), ms)` sleeps exactly like `setTimeout(resolve, ms)`.
  // Recursive, so `resolve.bind(a).bind(b)` does not launder it either.
  const bound = boundTarget(node);
  if (bound) return resolvesPromise(bound, resolveName);
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
      unboundWait:
        "`{{name}}()` is not imported from `e2e/helpers/causal-waits`, so this is an " +
        "undefined identifier that throws at runtime — on exactly the slow, loaded shards a " +
        "wait is there to survive. Import it, or use a causal wait instead.",
      promiseSleep:
        "Hand-rolled promise sleep is not a sanctioned wait. Use `dwell(ms, category, reason)` " +
        "from `e2e/helpers/causal-waits.ts`, or `injectLatency(ms, reason)` inside a " +
        "`page.route()` handler. See e2e/README.md.",
    },
  },
  create(context) {
    return {
      CallExpression(node) {
        // A sanctioned wait is only sanctioned if the identifier resolves to the
        // real helper. `eslint.config.mjs` turns `no-undef` OFF repo-wide, and
        // nothing typechecks `e2e/`, so an import that silently failed to apply
        // leaves `dwell(...)` linting clean and throwing at runtime — inside
        // whichever retry path only executes under load. Scope analysis catches
        // this without any type information, which is how `no-undef` works.
        if (node.callee.type === "Identifier" && SANCTIONED_WAITS.has(node.callee.name)) {
          const variable = lookupVariable(context.sourceCode.getScope(node), node.callee.name);
          if (!isCausalWaitsBinding(variable)) {
            context.report({
              node: node.callee,
              messageId: "unboundWait",
              data: { name: node.callee.name },
            });
          }
          return;
        }
        if (calleeMethodName(node.callee) !== WAIT_FOR_TIMEOUT) return;
        // Report at the METHOD token, not the whole call. A finding's line is
        // its node's start line, and the ratchet keeps only findings on lines
        // the change added. For a wrapped call —
        //
        //   page
        //     .waitForTimeout(500);
        //
        // — the CallExpression starts at `page`, so changing `.click(…)` to
        // `.waitForTimeout(…)` would attribute the finding to an UNCHANGED line
        // and the ratchet would silently drop it. The method token is on the
        // line that actually changed.
        const target = node.callee.type === "MemberExpression" ? node.callee.property : node.callee;
        context.report({ node: target, messageId: "waitForTimeout" });
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
