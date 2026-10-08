const RESOURCE_BY_FIELD = new Map([
  ["info", { resource: "About SystemInfo", replacement: "useSystemInfo" }],
  ["database", { resource: "database statistics", replacement: "useDatabaseStats" }],
  ["backups", { resource: "backup list", replacement: "useBackups" }],
  ["diskUsage", { resource: "disk usage", replacement: "useDiskUsage" }],
]);

const RESOURCE_BY_ACTION = new Map([
  ["setSystemInfo", RESOURCE_BY_FIELD.get("info")],
  ["setSystemDatabase", RESOURCE_BY_FIELD.get("database")],
  ["setSystemBackups", RESOURCE_BY_FIELD.get("backups")],
  ["setSystemDiskUsage", RESOURCE_BY_FIELD.get("diskUsage")],
]);

const MERGE_MODULE = "./merge-strategies";

function unwrap(node) {
  let current = node;
  while (
    current &&
    ["TSAsExpression", "TSTypeAssertion", "TSNonNullExpression", "ChainExpression"].includes(
      current.type,
    )
  ) {
    current = current.expression;
  }
  return current;
}

function staticKey(node, computed = false) {
  if (!computed && node?.type === "Identifier") return node.name;
  return node?.type === "Literal" && typeof node.value === "string" ? node.value : null;
}

function propertyKey(property) {
  return staticKey(property?.key, property?.computed === true);
}

function memberKey(member) {
  return staticKey(member?.property, member?.computed === true);
}

function membersOf(object) {
  const current = unwrap(object);
  return current?.type === "ObjectExpression" ? current.properties : [];
}

function propertyNamed(properties, name) {
  return properties.find(
    (property) => property.type !== "SpreadElement" && propertyKey(property) === name,
  );
}

function typeMembers(typeNode) {
  const current = unwrap(typeNode);
  return current?.type === "TSTypeLiteral" ? current.members : [];
}

function memberType(member) {
  return unwrap(member?.typeAnnotation?.typeAnnotation);
}

function lookupVariable(scope, name) {
  for (let current = scope; current; current = current.upper) {
    const found = current.variables.find((variable) => variable.name === name);
    if (found) return found;
  }
  return null;
}

function variableForIdentifier(sourceCode, node) {
  if (node?.type !== "Identifier") return null;
  return lookupVariable(sourceCode.getScope(node), node.name);
}

function sameBinding(sourceCode, reference, binding) {
  return Boolean(binding && variableForIdentifier(sourceCode, reference) === binding);
}

function isFunction(node) {
  return node?.type === "ArrowFunctionExpression" || node?.type === "FunctionExpression";
}

function walkBody(node, sourceCode, visit) {
  if (!node || typeof node.type !== "string") return;
  visit(node);
  if (isFunction(node)) return;
  for (const key of sourceCode.visitorKeys[node.type] ?? []) {
    const child = node[key];
    if (Array.isArray(child)) {
      for (const entry of child) walkBody(entry, sourceCode, visit);
    } else {
      walkBody(child, sourceCode, visit);
    }
  }
}

function topLevelDeclarations(program) {
  return program.body.flatMap((statement) =>
    statement.type === "ExportNamedDeclaration" && statement.declaration
      ? [statement.declaration]
      : [statement],
  );
}

function variableDeclarators(declaration) {
  return declaration?.type === "VariableDeclaration" ? declaration.declarations : [];
}

function functionBody(node) {
  const current = unwrap(node);
  if (!isFunction(current)) return null;
  if (current.body.type !== "BlockStatement") return current.body;
  const returned = current.body.body.find((statement) => statement.type === "ReturnStatement");
  return returned?.argument ?? null;
}

function directReturnedSetCall(actionValue, setBinding, sourceCode) {
  const returned = unwrap(functionBody(actionValue));
  if (returned?.type !== "CallExpression" || returned.callee.type !== "Identifier") return null;
  return sameBinding(sourceCode, returned.callee, setBinding) ? returned : null;
}

function systemField(target, rootBinding, sourceCode) {
  const field = unwrap(target);
  if (field?.type !== "MemberExpression") return null;
  const fieldName = memberKey(field);
  if (!RESOURCE_BY_FIELD.has(fieldName)) return null;

  const system = unwrap(field.object);
  if (system?.type !== "MemberExpression" || memberKey(system) !== "system") return null;
  if (!sameBinding(sourceCode, system.object, rootBinding)) return null;
  return { fieldName, node: field.property };
}

function findDeepMergeBinding(program, sourceCode) {
  for (const statement of program.body) {
    if (statement.type !== "ImportDeclaration" || statement.source.value !== MERGE_MODULE) continue;
    for (const specifier of statement.specifiers) {
      if (
        specifier.type === "ImportSpecifier" &&
        (specifier.imported.name ?? specifier.imported.value) === "deepMerge"
      ) {
        return variableForIdentifier(sourceCode, specifier.local);
      }
    }
  }
  return null;
}

function reportOwner(context, node, owner) {
  if (!owner || !node) return;
  context.report({
    node,
    messageId: "migratedSystemQueryOwner",
    data: owner,
  });
}

function reportFields(context, properties) {
  for (const property of properties) {
    if (property.type === "SpreadElement") continue;
    reportOwner(context, property, RESOURCE_BY_FIELD.get(propertyKey(property)));
  }
}

function checkTypes(context, declarations) {
  for (const declaration of declarations) {
    if (declaration.type !== "TSTypeAliasDeclaration") continue;

    if (declaration.id.name === "SystemBackupsState") {
      reportOwner(context, declaration.id, RESOURCE_BY_FIELD.get("backups"));
      continue;
    }

    if (declaration.id.name === "SystemSliceState") {
      const system = propertyNamed(typeMembers(declaration.typeAnnotation), "system");
      reportFields(context, typeMembers(memberType(system)));
    }

    if (declaration.id.name === "SystemSliceActions") {
      for (const member of typeMembers(declaration.typeAnnotation)) {
        reportOwner(context, member, RESOURCE_BY_ACTION.get(propertyKey(member)));
      }
    }
  }
}

function checkDefaultSystemState(context, declarator) {
  if (declarator.id?.type !== "Identifier" || declarator.id.name !== "defaultSystemState") return;
  const system = propertyNamed(membersOf(declarator.init), "system");
  reportFields(context, membersOf(system?.value));
}

function getReturnedObject(functionNode) {
  const returned = unwrap(functionBody(functionNode));
  return returned?.type === "ObjectExpression" ? returned : null;
}

function checkSystemSlice(context, sourceCode, declarator) {
  if (declarator.id?.type !== "Identifier" || declarator.id.name !== "createSystemSlice") return;
  const sliceFunction = unwrap(declarator.init);
  if (!isFunction(sliceFunction)) return;

  const returned = getReturnedObject(sliceFunction);
  if (!returned) return;

  const setParameter = sliceFunction.params[0];
  const setBinding = variableForIdentifier(sourceCode, setParameter);
  if (!setBinding) return;

  for (const action of returned.properties) {
    if (action.type === "SpreadElement") continue;
    const actionName = propertyKey(action);
    reportOwner(context, action, RESOURCE_BY_ACTION.get(actionName));

    const setCall = directReturnedSetCall(action.value, setBinding, sourceCode);
    const recipe = setCall?.arguments[0];
    if (!isFunction(recipe) || recipe.params[0]?.type !== "Identifier") continue;
    const draftParameter = recipe.params[0];

    walkBody(recipe.body, sourceCode, (node) => {
      if (node.type !== "AssignmentExpression" || node.operator !== "=") return;
      const owner = systemField(
        node.left,
        variableForIdentifier(sourceCode, draftParameter),
        sourceCode,
      );
      if (owner) reportOwner(context, owner.node, RESOURCE_BY_FIELD.get(owner.fieldName));
    });
  }
}

function isActualHydrateState(program, candidate) {
  if (candidate.type !== "FunctionDeclaration" || candidate.id?.name !== "hydrateState")
    return false;
  return program.body.some(
    (statement) =>
      statement.type === "ExportNamedDeclaration" && statement.declaration === candidate,
  );
}

function checkHydration(context, sourceCode, program, declaration) {
  if (!isActualHydrateState(program, declaration) || declaration.params[0]?.type !== "Identifier")
    return;
  const draftParameter = declaration.params[0];
  const draftBinding = variableForIdentifier(sourceCode, draftParameter);
  const deepMergeBinding = findDeepMergeBinding(program, sourceCode);
  if (!draftBinding) return;

  walkBody(declaration.body, sourceCode, (node) => {
    if (node.type === "AssignmentExpression" && node.operator === "=") {
      const owner = systemField(node.left, draftBinding, sourceCode);
      if (owner) reportOwner(context, owner.node, RESOURCE_BY_FIELD.get(owner.fieldName));
    }

    if (
      node.type !== "CallExpression" ||
      node.callee.type !== "Identifier" ||
      !sameBinding(sourceCode, node.callee, deepMergeBinding)
    ) {
      return;
    }

    const [target, payload] = node.arguments;
    const system = unwrap(target);
    if (
      system?.type !== "MemberExpression" ||
      memberKey(system) !== "system" ||
      !sameBinding(sourceCode, system.object, draftBinding)
    ) {
      return;
    }

    reportFields(context, membersOf(payload));
  });
}

function checkBarrelExports(context, program) {
  for (const statement of program.body) {
    if (statement.type !== "ExportNamedDeclaration" || statement.source?.value !== "./types") {
      continue;
    }
    for (const specifier of statement.specifiers) {
      if (
        specifier.type === "ExportSpecifier" &&
        (statement.exportKind === "type" || specifier.exportKind === "type") &&
        specifier.local.type === "Identifier" &&
        specifier.local.name === "SystemBackupsState"
      ) {
        reportOwner(context, specifier, RESOURCE_BY_FIELD.get("backups"));
      }
    }
  }
}

/** @type {import("eslint").Rule.RuleModule} */
export const noMigratedSystemQueryOwner = {
  meta: {
    type: "problem",
    docs: {
      description: "Keep migrated System Query snapshots out of Zustand state.",
    },
    schema: [],
    messages: {
      migratedSystemQueryOwner: "{{resource}} is Query-owned; read it with {{replacement}}.",
    },
  },
  create(context) {
    const sourceCode = context.sourceCode;
    return {
      "Program:exit"(program) {
        const declarations = topLevelDeclarations(program);
        checkTypes(context, declarations);
        for (const declaration of declarations) {
          for (const declarator of variableDeclarators(declaration)) {
            checkDefaultSystemState(context, declarator);
            checkSystemSlice(context, sourceCode, declarator);
          }
          checkHydration(context, sourceCode, program, declaration);
        }
        checkBarrelExports(context, program);
      },
    };
  },
};

export const systemQueryOwnerPlugin = {
  rules: { "no-migrated-system-zustand-owner": noMigratedSystemQueryOwner },
};
