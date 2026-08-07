/**
 * Turn a template literal carrying copy into an i18next message.
 *
 * `i18next/no-literal-string` only inspects plain literals, so a template
 * literal is invisible to the guard — these were the last shape holding
 * untranslated copy, mostly `aria-label`s, which screen readers do read aloud.
 *
 * Extracted from externalize-strings.mjs to keep that file under the 600-line
 * limit. `keyFor`/`qualify` are injected because they mutate the caller's
 * catalog state.
 */

const COERCIONS = new Set(["String", "Number", "Boolean"]);

/**
 * A name for the `{{placeholder}}` standing in for an interpolated expression.
 * Derived from the expression so the catalog reads `Remove {{label}}` rather
 * than `Remove {{value0}}` — a translator has to be able to tell what it is.
 */
export function placeholderName(expr, index) {
  const from = (node) => {
    switch (node.type) {
      case "Identifier":
        return node.name;
      case "MemberExpression":
        return node.property.type === "Identifier" ? node.property.name : null;
      case "TSNonNullExpression":
      case "TSAsExpression":
        return from(node.expression);
      case "LogicalExpression":
      case "BinaryExpression":
        // `index + 1` reads best as `{{index}}`, not `{{value0}}`.
        return from(node.left);
      case "CallExpression": {
        // `String(err)` / `Number(n)` are coercions — the argument is the
        // interesting name, not the wrapper.
        const callee = node.callee.type === "Identifier" ? node.callee.name : null;
        if (COERCIONS.has(callee) && node.arguments.length === 1) return from(node.arguments[0]);
        return from(node.callee);
      }
      default:
        return null;
    }
  };
  let raw = from(expr);
  if (!raw) return `value${index}`;
  // `getFileName(p)` / `formatBytes(n)` name the operation, not the value.
  const stripped = raw.replace(/^(get|format|render|to|compute)(?=[A-Z])/, "");
  if (stripped !== raw) raw = stripped[0].toLowerCase() + stripped.slice(1);
  // `a.b.name` and a bare `name` collide; the caller dedupes by suffixing.
  return raw.replace(/[^A-Za-z0-9]/g, "") || `value${index}`;
}

/** English words inside an interpolation are copy, not data — those need keys. */
function interpolationIsCopy(expr, { walk, looksLikeCopy }) {
  let copy = false;
  walk(expr, (n) => {
    if (n.type === "StringLiteral" && looksLikeCopy(n.value)) copy = true;
  });
  return copy;
}

/**
 * `` `Remove ${label}` `` -> the catalog message `Remove {{label}}` plus the
 * `{ label }` values object, or null when it should be left to a human.
 *
 * A template literal is invisible to `i18next/no-literal-string` (the rule only
 * inspects plain literals), so these were the last shape carrying untranslated
 * copy — mostly `aria-label`s, which screen readers do read aloud.
 */
export function templateToMessage(node, deps) {
  if (node.expressions.some((e) => e.type === "TemplateLiteral")) return null; // nested
  if (node.expressions.some((e) => interpolationIsCopy(e, deps))) return null; // branches are copy
  const prose = node.quasis.map((q) => q.value.cooked ?? "").join(" ");
  // Needs real words, not a path/URL/identifier fragment.
  if (!/[a-z]{3}/.test(prose) || !/[A-Za-z]{3}/.test(prose.replace(/[/:._-]/g, ""))) return null;
  if (/^[\s/:._-]*https?:/.test(prose)) return null;

  const used = new Map();
  const names = node.expressions.map((expr, i) => {
    const base = placeholderName(expr, i);
    const seen = used.get(base) ?? 0;
    used.set(base, seen + 1);
    return seen ? `${base}${seen + 1}` : base;
  });

  let message = "";
  node.quasis.forEach((q, i) => {
    message += q.value.cooked ?? "";
    if (i < names.length) message += `{{${names[i]}}}`;
  });
  message = message.trim();
  if (!message) return null;
  return { message, names };
}

/** Render the `t()` call for a converted template literal. */
export function templateCall(node, ns, { message, names }, source, { keyFor, qualify }) {
  const key = qualify(ns, keyFor(ns, message));
  if (!names.length) return `t("${key}")`;
  const values = names
    .map((name, i) => {
      const expr = source.slice(node.expressions[i].start, node.expressions[i].end);
      return expr === name ? name : `${name}: ${expr}`;
    })
    .join(", ");
  return `t("${key}", { ${values} })`;
}
