export type LspMonacoProviderMethod =
  | "provideCompletionItems"
  | "provideHover"
  | "provideDefinition"
  | "provideReferences"
  | "provideSignatureHelp";

export type LspProviderSupport = Readonly<{
  completion: boolean;
  hover: boolean;
  definition: boolean;
  references: boolean;
  signatureHelp: boolean;
}>;

function isOptions(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isBooleanOrOptions(value: unknown): boolean {
  return value === true || isOptions(value);
}

export function getLspProviderSupport(
  serverCapabilities: Record<string, unknown> | null,
): LspProviderSupport {
  return {
    completion: isOptions(serverCapabilities?.completionProvider),
    hover: isBooleanOrOptions(serverCapabilities?.hoverProvider),
    definition: isBooleanOrOptions(serverCapabilities?.definitionProvider),
    references: isBooleanOrOptions(serverCapabilities?.referencesProvider),
    signatureHelp: isOptions(serverCapabilities?.signatureHelpProvider),
  };
}

export function getLspMonacoProviderMethods(
  serverCapabilities: Record<string, unknown> | null,
): ReadonlySet<LspMonacoProviderMethod> {
  const support = getLspProviderSupport(serverCapabilities);
  const methods = new Set<LspMonacoProviderMethod>();
  if (support.completion) methods.add("provideCompletionItems");
  if (support.hover) methods.add("provideHover");
  if (support.definition) methods.add("provideDefinition");
  if (support.references) methods.add("provideReferences");
  if (support.signatureHelp) methods.add("provideSignatureHelp");
  return methods;
}
