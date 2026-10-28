import type { createRichOutputToolSchema } from "./schema";

type ToolSchema = ReturnType<typeof createRichOutputToolSchema>;
type Shape = {
  type?: string;
  enum?: string[];
  required?: string[];
  properties?: Record<string, Shape>;
  items?: Shape;
  anyOf?: Shape[];
};

// i18n-exempt: Agent-facing discovery policy, not browser copy.
const routing =
  "For structured comparisons, recorded checks or chart/graph/plot/preview/KPI/metrics with data: call directly.";
// i18n-exempt: Host presentation policy for agent discovery.
const presentation =
  "Kandev owns axes/legends/tooltips/layout/style. No ASCII/SVG/HTML/separate app. Markdown for small text tables. CSV: workspace-relative. Label series with units. line chart/bar chart/CSV chart; file preview.";

function objectFields(shape: Shape, includeEnumValues = false): string {
  return Object.entries(shape.properties ?? {})
    .map(([key, field]) => {
      const optional = shape.required?.includes(key) ? "" : "?";
      const values = includeEnumValues && field.enum?.length ? ":" + field.enum.join("|") : "";
      return key + optional + values;
    })
    .join(",");
}

function propertyShape(shape: Shape): string {
  if (shape.type === "array") return propertyShape(shape.items ?? {}) + "[]";
  if (shape.type === "object") return "{" + objectFields(shape, true) + "}";
  return shape.type ?? "value";
}

function fieldShape(key: string, shape: Shape): string {
  return key + ":" + propertyShape(shape);
}

function componentGuidance(name: string, props: Shape): string {
  const forms = props.anyOf ?? [props];
  const first = forms[0];
  const fields = first.properties ?? {};
  if (forms.length > 1) {
    const common = (first.required ?? []).filter((key) =>
      forms.every((form) => key in (form.properties ?? {})),
    );
    const sources = forms.map((form) =>
      (form.required ?? [])
        .filter((key) => !common.includes(key))
        .map((key) => fieldShape(key, form.properties![key]))
        .join(","),
    );
    return (
      name +
      " " +
      common.map((key) => fieldShape(key, fields[key])).join(",") +
      "; " +
      sources.join(" or ") +
      "."
    );
  }
  const keys = Object.keys(fields);
  const required = first.required ?? [];
  if (required.length === 1 && fields[required[0]].type === "array") {
    return name + " " + fieldShape(required[0], fields[required[0]]) + ".";
  }
  if (keys.length === 1 && fields[keys[0]].enum) {
    const values = JSON.stringify(fields[keys[0]].enum![0]);
    return name + " props:{" + keys[0] + ":" + values + "}.";
  }
  if (keys.length === 1) return name + " " + fieldShape(keys[0], fields[keys[0]]) + ".";
  return name + " props:{" + objectFields(first) + "}.";
}

export function createRichOutputDescription(schema: ToolSchema): string {
  const variants = schema.properties.spec.properties.elements.additionalProperties.oneOf;
  const components = ["Stack", "Table", "StatusList", "Callout"].map((name) => {
    const variant = variants.find((entry) => entry.properties.type.const === name);
    if (!variant) {
      // i18n-exempt: Catalog generation error, not browser copy.
      throw new Error("Missing discovery component: " + name);
    }
    return componentGuidance(name, variant.properties.props as Shape);
  });
  const elementKeys = variants[0].required.join(",");
  const version = schema.properties.version.const;
  const catalogVersion = schema.properties.catalog_version.const;
  // i18n-exempt: Generated shape guidance for the MCP tool contract.
  const envelope = `version:${version},catalog_version:${catalogVersion}; spec:{root,elements}; Each element: {${elementKeys}}; leaves children:[].`;
  const description = [routing, envelope, ...components, presentation].join(" ");
  if (Array.from(description).length > 650) {
    // i18n-exempt: Build error for overflowing agent discovery metadata.
    throw new Error("Rich-output discovery exceeds the 650-rune budget.");
  }
  return description;
}
