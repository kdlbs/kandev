/**
 * Decide whether a literal reached from the JSX is display copy, and in what
 * position — `"attr"` (a bare attribute value, so the replacement needs braces),
 * `"expr"` (already inside an expression), `"module-scope"`, or null for data.
 *
 * Extracted from externalize-strings.mjs to keep that file under the 600-line
 * limit. `DISPLAY_PROPS` / `DISPLAY_ATTRS` / `FUNCTION_TYPES` are injected
 * because they are that module's policy, not this one's.
 */

/**
 * A string literal is only display copy when something renders it. Literals
 * reached through these node types are data — comparisons, object/array keys,
 * enum arguments — and translating them changes behavior rather than wording.
 */
const NON_DISPLAY_PARENTS = new Set([
  "BinaryExpression",
  "SwitchCase",
  "ImportDeclaration",
  "ExportNamedDeclaration",
  "TSLiteralType",
  "TSEnumMember",
]);

let policy = { DISPLAY_PROPS: new Set(), DISPLAY_ATTRS: new Set(), FUNCTION_TYPES: new Set() };

/** Wire in the caller's display-prop policy before using displayContext(). */
export function configureDisplayContext(next) {
  policy = next;
}

/** True when the literal sits inside any function, i.e. not at module scope. */
const insideFunction = (ancestors) => ancestors.some((a) => policy.FUNCTION_TYPES.has(a.type));

/** Node types a literal may pass through on its way up to the JSX and still be copy. */
const PRESENTATIONAL = new Set([
  "ConditionalExpression",
  "LogicalExpression",
  "ParenthesizedExpression",
  "TSAsExpression",
  "TSNonNullExpression",
]);

/**
 * The nearest ancestor that actually decides what this literal is, skipping the
 * presentational wrappers (`cond ? … : …`, `err || …`, casts) the literal may sit
 * inside. Returns null when the chain passes through something that consumes the
 * string as data — a call argument, a comparison, a bare array element.
 */
function displayAnchor(node, ancestors) {
  for (let i = ancestors.length - 1; i >= 0; i--) {
    const a = ancestors[i];
    if (PRESENTATIONAL.has(a.type)) {
      // The *test* of a conditional is a comparison operand, not a rendered branch.
      if (a.type === "ConditionalExpression" && a.test === (ancestors[i + 1] ?? node)) return null;
      continue;
    }
    if (NON_DISPLAY_PARENTS.has(a.type)) return null;
    // Any call consumes the string as an argument rather than rendering it.
    if (a.type === "CallExpression" || a.type === "NewExpression") return null;
    return a;
  }
  return null;
}

/**
 * The JSX attribute that governs this literal, or null.
 *
 * Stops at the first JSXElement: once the literal is inside an element nested in
 * a prop (`actions={<><Button>Copy</Button></>}`), the element renders its own
 * children and the outer prop name says nothing about them.
 */
function governingAttribute(ancestors) {
  for (let i = ancestors.length - 1; i >= 0; i--) {
    const a = ancestors[i];
    if (a.type === "JSXElement" || a.type === "JSXFragment") return null;
    if (a.type === "JSXAttribute") return a;
  }
  return null;
}

/**
 * Where this literal lands: "attr" (a bare display prop), "expr" (inside a JSX
 * expression), "objprop" (a display key in a config object), "module-scope" (a
 * display key that would evaluate at import time), or null when it is data.
 *
 * These are the shapes `mode: "jsx-text-only"` could not see:
 *
 *   {saving ? "Saving..." : "Save"}      ->  {saving ? t("ns:saving") : t("ns:save")}
 *   <Field label="Assignee" />           ->  <Field label={t("ns:assignee")} />
 *   { label: "In progress" }             ->  { label: t("ns:inProgress") }
 */
const propName = (node) => String(node?.name?.name ?? node?.key?.name ?? node?.key?.value);

/** `label="Assignee"` — a bare literal on a display prop. */
function attributeContext(node, anchor) {
  if (!policy.DISPLAY_PROPS.has(propName(anchor))) return null;
  // A bare `label="x"` needs braces; a `label={cond ? "x" : "y"}` does not.
  return anchor.value === node ? "attr" : "expr";
}

/** `{ label: "In progress" }` — a display key in a config object. */
function objectPropertyContext(anchors, anchor) {
  if (!policy.DISPLAY_PROPS.has(propName(anchor))) return null;
  // A module-scope object evaluates its `t()` at import time, which pins the
  // copy to whatever locale was active then and never updates on a switch.
  return insideFunction(anchors) ? "objprop" : "module-scope";
}

/** `{saving ? "Saving..." : "Save"}` — inside a JSX expression container. */
function expressionContext(ancestors) {
  const attr = governingAttribute(ancestors);
  if (!attr) return "expr";
  const name = propName(attr);
  return policy.DISPLAY_PROPS.has(name) || policy.DISPLAY_ATTRS.has(name) ? "expr" : null;
}

export function displayContext(node, ancestors) {
  const anchor = displayAnchor(node, ancestors);
  switch (anchor?.type) {
    case "JSXAttribute":
      return attributeContext(node, anchor);
    case "ObjectProperty":
      return objectPropertyContext(ancestors, anchor);
    case "JSXExpressionContainer":
      return expressionContext(ancestors);
    default:
      return null;
  }
}
