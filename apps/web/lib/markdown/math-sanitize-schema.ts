import { defaultSchema, type Options as SanitizeSchema } from "rehype-sanitize";

type PropertyDefinition = NonNullable<NonNullable<SanitizeSchema["attributes"]>[string]>[number];
type ClassNameDefinition = [string, ...(string | number | boolean | RegExp | null | undefined)[]];

const mathClassNames = ["math-inline", "math-display"] as const;

function isClassNameDefinition(definition: PropertyDefinition): definition is ClassNameDefinition {
  return Array.isArray(definition) && definition[0] === "className";
}

/** Extend an existing schema with only the marker classes consumed by rehype-katex. */
export function withMarkdownMathSanitizeSchema(schema: SanitizeSchema): SanitizeSchema {
  const codeAttributes = schema.attributes?.code ?? [];
  const existingClassNameDefinition = codeAttributes.find(isClassNameDefinition);
  const existingClassNameValues = existingClassNameDefinition?.slice(1) ?? [];
  const codeAttributesWithoutClassName = codeAttributes.filter(
    (definition) => !isClassNameDefinition(definition),
  );
  const mergedClassNameDefinition = [
    "className",
    ...existingClassNameValues,
    ...mathClassNames,
  ] as PropertyDefinition;

  return {
    ...schema,
    attributes: {
      ...schema.attributes,
      code: [mergedClassNameDefinition, ...codeAttributesWithoutClassName],
    },
  };
}

export const markdownMathSanitizeSchema = withMarkdownMathSanitizeSchema(defaultSchema);
