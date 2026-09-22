/**
 * Keep first-party task workbench navigation on the shared URL and link
 * authorities. This rule deliberately proves only the common static forms:
 * literal/template/concatenated task routes, simple local aliases, imported
 * linkToTask aliases, native task anchors, and window.location writes.
 */

const TASK_ROUTE_PREFIX = /^\/(?:t|tasks)\//;
const TASK_ROUTE_WITH_ID = /^\/(?:t|tasks)\/.+/;
const LINKS_MODULE = /(?:^|\/)lib\/links(?:\.[cm]?[jt]s)?$/;

function unwrap(node) {
  if (!node) return null;
  if (
    node.type === "TSAsExpression" ||
    node.type === "TSTypeAssertion" ||
    node.type === "TSNonNullExpression" ||
    node.type === "ChainExpression"
  ) {
    return unwrap(node.expression);
  }
  return node;
}

function lookupVariable(scope, name) {
  for (let current = scope; current; current = current.upper) {
    const found = current.variables.find((variable) => variable.name === name);
    if (found) return found;
  }
  return null;
}

function variableForIdentifier(context, node) {
  if (node.type !== "Identifier") return null;
  return lookupVariable(context.sourceCode.getScope(node), node.name);
}

function isLinksImport(def) {
  return def.type === "ImportBinding" && LINKS_MODULE.test(String(def.parent?.source?.value));
}

function isLinkToTaskImport(def) {
  if (!isLinksImport(def)) return false;
  return def.node?.type === "ImportSpecifier" && def.node.imported?.name === "linkToTask";
}

function isLinksNamespaceImport(def) {
  return isLinksImport(def) && def.node?.type === "ImportNamespaceSpecifier";
}

function resolvesToLinkToTask(context, node, seen = new Set()) {
  const current = unwrap(node);
  if (!current) return false;

  if (current.type === "MemberExpression") {
    const property = current.computed ? current.property : current.property;
    const propertyName = property.type === "Identifier" ? property.name : property.value;
    if (propertyName !== "linkToTask") return false;
    const namespace = current.object;
    if (namespace.type !== "Identifier") return false;
    const variable = variableForIdentifier(context, namespace);
    return Boolean(variable?.defs.some(isLinksNamespaceImport));
  }

  if (current.type !== "Identifier") return false;
  const variable = variableForIdentifier(context, current);
  if (!variable || seen.has(variable)) return false;
  seen.add(variable);
  if (variable.defs.some(isLinkToTaskImport)) return true;

  return variable.defs.some((def) => {
    if (def.type !== "Variable" || !def.node?.init) return false;
    return resolvesToLinkToTask(context, def.node.init, seen);
  });
}

function staticStringFromVariable(context, node, seen) {
  const variable = variableForIdentifier(context, node);
  if (!variable || seen.has(variable)) return null;
  for (const def of variable.defs) {
    if (def.type !== "Variable" || !def.node?.init) continue;
    const branchSeen = new Set(seen);
    branchSeen.add(variable);
    const value = isStaticString(context, def.node.init, branchSeen);
    if (value !== null) return value;
  }
  return null;
}

function isStaticString(context, node, seen = new Set()) {
  const current = unwrap(node);
  if (!current) return null;
  if (current.type === "Literal" && typeof current.value === "string") return current.value;
  if (current.type === "TemplateLiteral" && current.expressions.length === 0) {
    return current.quasis[0]?.value.cooked ?? "";
  }
  if (current.type === "BinaryExpression" && current.operator === "+") {
    const left = isStaticString(context, current.left, new Set(seen));
    const right = isStaticString(context, current.right, new Set(seen));
    return left !== null && right !== null ? left + right : null;
  }
  return current.type === "Identifier" ? staticStringFromVariable(context, current, seen) : null;
}

function routeFromTemplate(node) {
  const first = node.quasis[0]?.value.cooked ?? "";
  if (!TASK_ROUTE_PREFIX.test(first)) return null;
  if (TASK_ROUTE_WITH_ID.test(first)) return "raw";
  return node.expressions.length > 0 ? "raw" : null;
}

function routeFromBinary(context, node, seen) {
  const staticValue = isStaticString(context, node, new Set(seen));
  if (staticValue !== null) return TASK_ROUTE_WITH_ID.test(staticValue) ? "raw" : null;
  const left = isStaticString(context, node.left, new Set(seen));
  if (!left || !TASK_ROUTE_PREFIX.test(left)) return null;
  if (TASK_ROUTE_WITH_ID.test(left)) return "raw";
  // A dynamic right-hand side supplies the task segment. A static query or
  // hash after the bare prefix still has no task id and is not a detail route.
  const right = isStaticString(context, node.right, new Set(seen));
  return right === null ? "raw" : null;
}

function combineRouteKinds(left, right) {
  if (left === "raw" || right === "raw") return "raw";
  if (left === "builder" || right === "builder") return "builder";
  return null;
}

function routeKindFromVariable(context, node, seen) {
  const variable = variableForIdentifier(context, node);
  if (!variable || seen.has(variable)) return null;
  if (variable.defs.some(isLinkToTaskImport)) return "builder";
  for (const def of variable.defs) {
    if (def.type !== "Variable" || !def.node?.init) continue;
    const branchSeen = new Set(seen);
    branchSeen.add(variable);
    const value = routeKind(context, def.node.init, branchSeen);
    if (value) return value;
  }
  return null;
}

function routeKindFromComposite(context, node, seen) {
  const left = routeKind(context, node.left ?? node.consequent, new Set(seen));
  const right = routeKind(context, node.right ?? node.alternate, new Set(seen));
  return combineRouteKinds(left, right);
}

function routeKind(context, node, seen = new Set()) {
  const current = unwrap(node);
  if (!current) return null;
  switch (current.type) {
    case "Literal":
      return typeof current.value === "string" && TASK_ROUTE_WITH_ID.test(current.value)
        ? "raw"
        : null;
    case "TemplateLiteral":
      return routeFromTemplate(current);
    case "BinaryExpression":
      return current.operator === "+" ? routeFromBinary(context, current, seen) : null;
    case "ConditionalExpression":
    case "LogicalExpression":
      return routeKindFromComposite(context, current, seen);
    case "SequenceExpression":
      return routeKind(context, current.expressions.at(-1), seen);
    case "CallExpression":
      return resolvesToLinkToTask(context, current.callee) ? "builder" : null;
    case "Identifier":
      return routeKindFromVariable(context, current, seen);
    default:
      return null;
  }
}

function isNativeAnchorHref(node) {
  const opening = node.parent;
  return (
    opening?.type === "JSXOpeningElement" &&
    opening.name?.type === "JSXIdentifier" &&
    opening.name.name === "a"
  );
}

function hrefExpression(node) {
  if (!node) return null;
  if (node.type === "JSXExpressionContainer") return node.expression;
  return node;
}

function isLocationObject(node) {
  const current = unwrap(node);
  if (!current) return false;
  if (current.type === "Identifier") return current.name === "location";
  return (
    current.type === "MemberExpression" &&
    !current.computed &&
    current.property.type === "Identifier" &&
    current.property.name === "location" &&
    current.object.type === "Identifier" &&
    (current.object.name === "window" || current.object.name === "globalThis")
  );
}

function isLocationHref(node) {
  const current = unwrap(node);
  return (
    current?.type === "MemberExpression" &&
    ((current.computed &&
      current.property.type === "Literal" &&
      current.property.value === "href") ||
      (!current.computed &&
        current.property.type === "Identifier" &&
        current.property.name === "href")) &&
    isLocationObject(current.object)
  );
}

function isLocationMethod(node) {
  const current = unwrap(node);
  if (current?.type !== "MemberExpression" || !isLocationObject(current.object)) return false;
  const property = current.computed ? current.property : current.property;
  const name = property.type === "Identifier" ? property.name : property.value;
  return name === "assign" || name === "replace";
}

function isRouteContainer(parent, node) {
  if (parent.type === "BinaryExpression" && parent.operator === "+") return true;
  if (parent.type === "ConditionalExpression") {
    return parent.consequent === node || parent.alternate === node;
  }
  if (parent.type === "LogicalExpression") return parent.right === node;
  if (parent.type === "TSAsExpression") return parent.expression === node;
  if (parent.type === "TSTypeAssertion") return parent.expression === node;
  if (parent.type === "TSNonNullExpression") return parent.expression === node;
  return parent.type === "ChainExpression" && parent.expression === node;
}

function isNestedRoutePart(node, context) {
  const parent = node.parent;
  return Boolean(parent && isRouteContainer(parent, node) && routeKind(context, parent) === "raw");
}

/** @type {import("eslint").Rule.RuleModule} */
export const noTaskLinkBypass = {
  meta: {
    type: "problem",
    docs: {
      description: "Require shared task URL and link primitives for task workbench navigation.",
    },
    schema: [],
    messages: {
      taskLinkBypass:
        "Use linkToTask() for task workbench URLs and TaskLink/AppLink for task anchors; do not hand-build or natively navigate task-detail links.",
    },
  },
  create(context) {
    const reported = new WeakSet();
    const report = (node) => {
      if (!node || reported.has(node)) return;
      reported.add(node);
      context.report({ node, messageId: "taskLinkBypass" });
    };

    return {
      JSXAttribute(node) {
        if (node.name?.type !== "JSXIdentifier" || node.name.name !== "href") return;
        const expression = hrefExpression(node.value);
        const kind = routeKind(context, expression);
        if (!kind) return;
        if (kind === "builder" && !isNativeAnchorHref(node)) return;
        // Local raw aliases are reported at their defining expression. This
        // avoids a second diagnostic on the consuming JSX attribute.
        if (kind === "raw" && expression?.type === "Identifier") return;
        report(expression ?? node);
      },
      AssignmentExpression(node) {
        if (isLocationHref(node.left) && routeKind(context, node.right)) report(node.right);
      },
      CallExpression(node) {
        if (isLocationMethod(node.callee) && routeKind(context, node.arguments[0])) {
          report(node.arguments[0]);
        }
      },
      Literal(node) {
        if (typeof node.value !== "string" || isNestedRoutePart(node, context)) return;
        if (node.parent?.type === "JSXAttribute") return;
        if (routeKind(context, node) === "raw") report(node);
      },
      TemplateLiteral(node) {
        if (isNestedRoutePart(node, context)) return;
        if (routeKind(context, node) === "raw") report(node);
      },
      BinaryExpression(node) {
        if (node.operator !== "+" || isNestedRoutePart(node, context)) return;
        if (routeKind(context, node) === "raw") report(node);
      },
      ConditionalExpression(node) {
        if (routeKind(context, node) === "raw") report(node);
      },
      LogicalExpression(node) {
        if (routeKind(context, node) === "raw") report(node);
      },
    };
  },
};

export const taskLinksPlugin = {
  rules: { "no-task-link-bypass": noTaskLinkBypass },
};

export const TASK_LINK_RULE_ID = "task-links/no-task-link-bypass";
