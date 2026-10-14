import { z } from "zod";
import { catalog } from "./catalog";
import { richOutputExamples } from "./examples";

const idSchema = {
  type: "string",
  pattern: "^[A-Za-z][A-Za-z0-9_-]{0,63}$",
  not: { enum: ["constructor", "prototype", "__proto__"] },
};

export function createRichOutputToolSchema() {
  const variants = Object.entries(catalog.data.components).map(([name, component]) => {
    const { $schema: _dialect, ...props } = z.toJSONSchema(component.props);
    const container = "slots" in component;
    return {
      type: "object",
      description: component.description,
      examples: richOutputExamples
        .flatMap((example) => Object.values(example.spec.elements))
        .filter((element) => element.type === name),
      properties: {
        type: { const: name },
        props,
        children: {
          type: "array",
          items: idSchema,
          minItems: container ? 1 : 0,
          maxItems: container ? 64 : 0,
        },
      },
      required: ["type", "props", "children"],
      additionalProperties: false,
    };
  });
  return {
    type: "object",
    // i18n-exempt: Agent-facing composition limits for MCP discovery.
    description:
      "One literal tree: at most four Chart, Table, StatusList, Metrics or FilePreview data components. Callout and Text are explanatory. At most eight levels; every element must be reachable exactly once.",
    examples: richOutputExamples,
    properties: {
      version: { type: "integer", const: 2 },
      catalog_version: { type: "integer", const: 1 },
      title: { type: "string", minLength: 1, maxLength: 120 },
      description: { type: "string", maxLength: 500 },
      spec: {
        type: "object",
        properties: {
          root: idSchema,
          elements: {
            type: "object",
            minProperties: 1,
            maxProperties: 64,
            propertyNames: idSchema,
            additionalProperties: { oneOf: variants },
          },
        },
        required: ["root", "elements"],
        additionalProperties: false,
      },
    },
    required: ["version", "catalog_version", "title", "spec"],
    additionalProperties: false,
  };
}
