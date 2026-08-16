/**
 * Find user-facing copy sitting in a position `i18next/no-literal-string`
 * structurally cannot see.
 *
 * That rule runs in `jsx-only` mode, so it inspects literals in JSX and nothing
 * else. docs/i18n.md enumerates what falls through, and every shape below was
 * found in already-migrated code that lint, `i18n:check`, the ratchet and the
 * pseudo-locale walk all reported clean:
 *
 *   const ROWS = [{ label: "Disk usage" }]         // SCREAMING_CASE is skipped
 *   function badge() { return "Waiting for input" }  // a plain helper holds no JSX
 *   function C({ label = "Running..." }) {}        // evaluated before the hook
 *   toast({ title: "Rename failed" })              // never a JSX literal
 *
 * The last one is the expensive class: the string is handed to a TRANSLATED
 * frame, so under the pseudo-locale it renders inside accented copy and reads as
 * done. `t("common:gitOperationFailed", { operation: "Stage all" })` is the
 * worked example — its five sibling call sites pass a `t()` result.
 *
 * Scope is the same contract as the eslint guard: callers pass the files to
 * judge, and `check-nonjsx-copy.mjs` passes `i18nGuardFiles`. Judging only
 * already-migrated paths is what keeps this from becoming the repo-wide treadmill
 * that killed the first migration attempt (docs/i18n.md, "The guard is scoped").
 *
 * A finding is silenced with `// i18n-exempt: <reason>` on the enclosing
 * statement. The reason is mandatory — the shapes this finds are genuinely
 * ambiguous (a persisted default, a prompt sent verbatim to a model, a wire
 * value), and the next reader needs to know which one it is without re-deriving
 * it. See `exemptionsFor`.
 */
import { parseSync } from "@babel/core";

/** Property names whose value is displayed rather than used as data. */
const COPY_KEYS = new Set([
  "alt",
  "ariaLabel",
  "aria-label",
  "body",
  "caption",
  "cancelText",
  "confirmText",
  "description",
  "emptyMessage",
  "emptyText",
  "errorMessage",
  "heading",
  "helpText",
  "hint",
  "label",
  "message",
  "placeholder",
  "subtitle",
  "successMessage",
  "summary",
  "text",
  "title",
  "tooltip",
]);

const FUNCTION_TYPES = new Set([
  "ArrowFunctionExpression",
  "ClassMethod",
  "FunctionDeclaration",
  "FunctionExpression",
  "ObjectMethod",
]);

/**
 * Calls whose string arguments reach the user directly.
 *
 * The `set*` half is matched by SHAPE rather than by an enumerated list. Every
 * screen names its own state setter — `setPreviewError`, `setSaveWarning`,
 * `setJoinStatus` — so a fixed list only ever catches the ones someone
 * remembered to add, and the first file this check was pointed at held a
 * `setPreviewError` two lines above a name that was on the list.
 */
const DISPLAY_CALLS =
  /^(alert|confirm|prompt|toast|toast\.\w+|window\.(alert|confirm|prompt)|set[A-Za-z]*(Error|Message|Notice|Warning|Status|Label|Text|Title|Reason|Summary|Hint))$/;

/**
 * Identifiers that hold styling rather than prose. Tailwind class lists are the
 * single largest source of false positives here — they are long, spaced, and
 * word-like — and they arrive through exactly these names.
 */
const STYLE_NAME = /(class(name)?|style|css|selector|query|variant|token)s?$/i;

/**
 * Strings that are not translatable copy even when they read like prose.
 *
 * Deliberately narrow. Anything judgement-shaped (a persisted default, an
 * agent-facing prompt) is NOT listed here — it takes an `i18n-exempt` comment
 * with a reason, so the decision stays visible at the call site instead of being
 * silently absorbed by a pattern in this file.
 */
const NOT_COPY = [
  /^\s*$/,
  /^[^A-Za-z]*$/,
  // CSS: selectors, media queries, declarations, and class lists.
  /^[.#[(]/,
  /^[a-z-]+\s*:\s*[^;]+$/i,
  /(^|\s)(flex|grid|hidden|absolute|relative|sticky|fixed|inline-\w+|block|truncate|overflow-\w+|shrink|grow|group|peer)(\s|$)/,
  /(^|\s)[a-z-]+-(\[|\d|auto|full|px|none|foreground|background|muted|primary|secondary|border|card)/,
  // A class list, not a lowercase sentence. Rejecting ALL lowercase multi-word
  // strings would be the easy version and it hides real copy: "every hour",
  // "ended (session restart)" and "watching" are all lowercase and all shown to
  // a user. Require instead that EVERY word be hyphenated or a bare CSS token,
  // which no English phrase is.
  /^[a-z][\w-]*(-[\w-]+)+(\s+[a-z][\w-]*(-[\w-]+)+)*$/,
  // Identifiers, keys, paths, and wire values.
  /^[a-z][A-Za-z0-9]*$/,
  /^[A-Z][A-Z0-9_]*$/,
  /^[\w.-]+:[\w.-]+$/,
  /^[~./]/,
  /^(https?|file|ssh|git|data|mailto):/,
  // Log and debug prefixes are developer output, never shown to a user.
  /^\[[A-Za-z][\w\s]*\]/,
  // Single short word: too little signal to call it copy either way.
  /^\S{1,3}$/,
];

/**
 * True when `value` reads as something a user is meant to understand.
 *
 * @param {string} value
 * @param {string[]} [extraExcludes]
 */
export function looksLikeCopy(value, extraExcludes = []) {
  if (typeof value !== "string") return false;
  const text = value.trim();
  if (!text) return false;
  if (NOT_COPY.some((pattern) => pattern.test(text))) return false;
  // The eslint guard's own exclusions, so the two agree on what a brand noun,
  // acronym, unit or keyboard glyph is. The plugin anchors every entry.
  if (extraExcludes.some((pattern) => new RegExp(`^${pattern}$`).test(text))) return false;
  const words = text.split(/\s+/).filter((word) => /[A-Za-z]{2,}/.test(word));
  if (words.length >= 2) return true;
  // One word only counts when it is capitalized prose ("Staged", "Walkthrough").
  return /^[A-Z][a-z]{3,}[.!?]?$/.test(text);
}

/**
 * The copy a literal carries, or null when it carries none.
 *
 * A template literal is judged on its LONGEST static chunk, not on the chunks
 * joined: `` `Every day at ${at} ${zone}` `` is copy because "Every day at "
 * is, while `` `${w}px ${h}em` `` is not — joining would splice "px" and "em"
 * into a two-word phrase and report a CSS value as a sentence.
 */
function literalCopy(node, excludes) {
  if (node.type === "StringLiteral") {
    return looksLikeCopy(node.value, excludes) ? node.value : null;
  }
  if (node.type !== "TemplateLiteral") return null;
  const chunks = node.quasis.map((quasi) => quasi.value.cooked ?? quasi.value.raw);
  const longest = chunks.reduce((a, b) => (b.trim().length > a.trim().length ? b : a), "");
  if (!looksLikeCopy(longest, excludes)) return null;
  return chunks.join("${…}");
}

function walk(node, visit, ancestors = []) {
  if (!node || typeof node.type !== "string") return;
  visit(node, ancestors);
  const next = [...ancestors, node];
  for (const key of Object.keys(node)) {
    if (key === "loc" || key.endsWith("Comments")) continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (const item of child) if (item && typeof item.type === "string") walk(item, visit, next);
    } else if (child && typeof child.type === "string") {
      walk(child, visit, next);
    }
  }
}

function nameOf(node) {
  if (!node) return "";
  if (node.type === "Identifier") return node.name;
  if (node.type === "StringLiteral") return node.value;
  return "";
}

/**
 * The names a call could be recognised by: the whole callee, and — for a member
 * call — the property on its own.
 *
 * The bare property matters because setters are routinely passed around in a
 * bag: `setters.setError("Failed to resume session")` is the same display call
 * as `setError(...)`, and matching only the dotted form misses it. This file's
 * first run over `use-session-resumption.ts` caught the free-function call two
 * lines from a `setters.` one and reported only the former.
 */
function calleeNames(callee) {
  if (!callee) return [];
  if (callee.type === "Identifier") return [callee.name];
  if (callee.type === "MemberExpression") {
    const property = nameOf(callee.property);
    return [`${nameOf(callee.object)}.${property}`, property];
  }
  return [];
}

/** The function a node sits in, and the name it was declared under. */
function enclosingFunction(ancestors) {
  for (let index = ancestors.length - 1; index >= 0; index -= 1) {
    const node = ancestors[index];
    if (!FUNCTION_TYPES.has(node.type)) continue;
    const parent = ancestors[index - 1];
    const name =
      node.id?.name ||
      (parent?.type === "VariableDeclarator" ? nameOf(parent.id) : "") ||
      (parent?.type === "ObjectProperty" ? nameOf(parent.key) : "");
    return { node, name };
  }
  return null;
}

/**
 * The nodes an exemption comment may attach to, innermost first.
 *
 * Two granularities, because the legitimate cases come in two sizes. A
 * `const TABLE = [...]` is one decision, so a comment above the declaration
 * covers every row. A prompt builder is also one decision, but its strings sit
 * in `return` statements inside the function — attaching only to the nearest
 * statement would need a marker on each, repeating the same sentence four times
 * and inviting whoever adds the fifth to skip it.
 *
 * So the enclosing FUNCTION counts too. That is broader than it looks safe, and
 * it is bounded by the reason being mandatory: silencing a whole component takes
 * writing down why, in a diff a reviewer reads.
 */
function exemptionTargets(ancestors) {
  const targets = [];
  for (let index = ancestors.length - 1; index >= 0; index -= 1) {
    const candidate = ancestors[index];
    if (/(Statement|Declaration)$/.test(candidate.type) || FUNCTION_TYPES.has(candidate.type)) {
      targets.push(candidate);
    }
  }
  return targets;
}

/**
 * Lines carrying `// i18n-exempt: <reason>`, mapped to the reason.
 *
 * A marker with no reason after the colon is reported rather than honoured — an
 * unexplained exemption is the thing this check exists to prevent.
 */
export function exemptionsFor(comments) {
  const withReason = new Set();
  const withoutReason = [];
  for (const comment of comments ?? []) {
    const match = /i18n-exempt:?(.*)$/.exec(comment.value);
    if (!match) continue;
    const line = comment.loc.start.line;
    if (match[1].trim().length >= 3) withReason.add(line);
    else withoutReason.push(line);
  }
  return { withReason, withoutReason };
}

/** Positions this check never judges, whatever the literal says. */
function isJudgeablePosition(node, parent, ancestors) {
  // JSX belongs to the eslint rule; reporting it here would double every
  // finding on a guarded path.
  if (ancestors.some((ancestor) => ancestor.type.startsWith("JSX"))) return false;
  // Types, imports and enum members are never rendered.
  if (/^(TS|Import|Export)/.test(parent.type)) return false;
  // An object KEY is an identifier even when it is quoted.
  return !(parent.type === "ObjectProperty" && parent.key === node);
}

/** Whether a literal's position is one this check judges, and under what name. */
function classify(node, ancestors) {
  const parent = ancestors[ancestors.length - 1];
  if (!parent || !isJudgeablePosition(node, parent, ancestors)) return null;

  const propertyKey =
    parent.type === "ObjectProperty" && parent.value === node ? nameOf(parent.key) : null;
  const screaming = ancestors.find(
    (ancestor) =>
      ancestor.type === "VariableDeclarator" &&
      ancestor.id?.type === "Identifier" &&
      /^[A-Z][A-Z0-9_]{2,}$/.test(ancestor.id.name),
  );

  // Every prose-shaped value in a config table, not only the slots named `label`
  // or `title`. A lookup table keys its copy by an ENUM — `INTERVAL_LABELS` is
  // `{ "every-5m": "Every 5 minutes" }` — so a copy-key filter reads the key
  // `every-5m`, decides the slot is data, and skips the sentence in it.
  // `looksLikeCopy` already rejects ids, so the ids and icon names sitting
  // beside the copy never reach here.
  if (screaming && !STYLE_NAME.test(screaming.id.name)) {
    const suffix = propertyKey ? `.${propertyKey}` : "";
    return { kind: "config-table", context: `${screaming.id.name}${suffix}` };
  }
  if (propertyKey && COPY_KEYS.has(propertyKey)) {
    return { kind: "copy-property", context: propertyKey };
  }
  if (parent.type === "CallExpression") {
    const name = calleeNames(parent.callee).find((candidate) => DISPLAY_CALLS.test(candidate));
    if (name) return { kind: "display-call", context: name };
  }
  if (parent.type === "AssignmentPattern" && parent.right === node) {
    return isParameterDefault(parent, ancestors)
      ? { kind: "param-default", context: nameOf(parent.left) }
      : null;
  }
  return classifyReturn(node, parent, ancestors);
}

/** A default in a function's parameter list, which runs before the body. */
function isParameterDefault(pattern, ancestors) {
  const fn = enclosingFunction(ancestors);
  return Boolean(fn?.node.params?.some((param) => ancestors.includes(param) || param === pattern));
}

/**
 * A string handed back by a plain helper.
 *
 * Components are excluded by name: they return JSX, which the eslint rule
 * already covers, and their `className` ternaries would otherwise all land here.
 */
function classifyReturn(node, parent, ancestors) {
  const returnish =
    parent.type === "ReturnStatement" ||
    (parent.type === "ArrowFunctionExpression" && parent.body === node) ||
    ((parent.type === "ConditionalExpression" || parent.type === "LogicalExpression") &&
      parent.test !== node);
  if (!returnish) return null;
  const fn = enclosingFunction(ancestors);
  if (!fn || /^[A-Z]/.test(fn.name) || STYLE_NAME.test(fn.name)) return null;
  const declarator = ancestors.find(
    (ancestor) => ancestor.type === "VariableDeclarator" && ancestor.id?.type === "Identifier",
  );
  if (declarator && STYLE_NAME.test(nameOf(declarator.id))) return null;
  return { kind: "helper-return", context: fn.name };
}

/**
 * Copy in a non-JSX position, one entry per string.
 *
 * `source` is parsed as TS/TSX. Anything that fails to parse yields no findings
 * rather than throwing: this runs over whole directories, and a gate that dies on
 * one unparseable file reports nothing about the other fourteen hundred.
 *
 * Returns `{ findings, markersWithoutReason }` on every path, including that one.
 *
 * @param {string} source
 * @param {{ filename?: string, excludes?: string[] }} [options]
 */
export function findNonJsxCopy(source, options = {}) {
  const { filename = "input.tsx", excludes = [] } = options;
  let ast;
  try {
    ast = parseSync(source, {
      filename,
      babelrc: false,
      configFile: false,
      sourceType: "module",
      parserOpts: { plugins: ["typescript", "jsx"] },
    });
  } catch {
    // Same shape as a clean parse: callers destructure `{ findings }`, and
    // handing them a bare array here would throw on the file after the broken
    // one rather than skipping it.
    return { findings: [], markersWithoutReason: [] };
  }

  const { withReason, withoutReason } = exemptionsFor(ast.comments);
  const findings = [];
  const seen = new Set();

  const report = (node, ancestors, kind, context, value) => {
    const line = node.loc.start.line;
    // A marker on the line above any enclosing statement or function — through
    // to the finding itself — silences it.
    for (const target of exemptionTargets(ancestors)) {
      for (let candidate = target.loc.start.line - 1; candidate <= line; candidate += 1) {
        if (withReason.has(candidate)) return;
      }
    }
    const id = `${line}:${node.loc.start.column}`;
    if (seen.has(id)) return;
    seen.add(id);
    findings.push({ line, column: node.loc.start.column, kind, context, value });
  };

  walk(ast.program, (node, ancestors) => {
    const value = literalCopy(node, excludes);
    if (value === null) return;
    const hit = classify(node, ancestors);
    if (hit) report(node, ancestors, hit.kind, hit.context, value);
  });

  return { findings, markersWithoutReason: withoutReason };
}
