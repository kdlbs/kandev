import { expect, it } from "vitest";
import { createRichOutputToolSchema } from "./schema";
import { createRichOutputDescription } from "./guidance";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.11
it("derives metadata from changed schema fields instead of a fixed recipe", () => {
  const schema = createRichOutputToolSchema();
  const table = schema.properties.spec.properties.elements.additionalProperties.oneOf.find(
    (variant) => variant.properties.type.const === "Table",
  )!;
  const props = table.properties.props as unknown as {
    anyOf: Array<{
      properties: {
        columns: { items: { properties: Record<string, unknown>; required: string[] } };
      };
    }>;
  };
  for (const form of props.anyOf) {
    form.properties.columns.items.properties.unit = { type: "string" };
    form.properties.columns.items.required.push("unit");
  }
  expect(createRichOutputDescription(schema)).toContain("columns:{key,label,unit}[]");
});

it("fails discovery generation on overflow without truncating shapes", () => {
  const schema = createRichOutputToolSchema();
  schema.properties.spec.properties.elements.additionalProperties.oneOf[0].required.push(
    "field".repeat(150),
  );
  expect(() => createRichOutputDescription(schema)).toThrow("650-rune budget");
});
