import { expect, it, vi } from "vitest";
import { catalog, componentProps, type ComponentName } from "./catalog";
import { createRichOutputToolSchema } from "./schema";
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2, AC-AGENTS-AGENT-RICH-OUTPUT-002.10
it("exports actual component property contracts for backend validation", () => {
  const schema = createRichOutputToolSchema();
  expect(JSON.stringify(schema)).toContain('"x_column"');
  expect(JSON.stringify(schema)).not.toContain('"visible"');
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.11
it("provides complete component-local examples from valid reports", () => {
  const variants =
    createRichOutputToolSchema().properties.spec.properties.elements.additionalProperties.oneOf;
  for (const variant of variants) {
    const examples = (
      variant as unknown as {
        examples: Array<{ type: string; props: unknown; children: string[] }>;
      }
    ).examples;
    expect(examples, variant.properties.type.const).toBeInstanceOf(Array);
    expect(examples.length).toBeGreaterThan(0);
    for (const example of examples) {
      expect(example.type).toBe(variant.properties.type.const);
      expect(componentProps[example.type as ComponentName].safeParse(example.props).success).toBe(
        true,
      );
      expect(Array.isArray(example.children)).toBe(true);
    }
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2
it("keeps future library fields outside the closed agent spec", () => {
  const exported = catalog.jsonSchema({ strict: false }) as {
    properties: Record<string, unknown>;
  };
  const spy = vi.spyOn(catalog, "jsonSchema").mockReturnValue({
    ...exported,
    properties: { ...exported.properties, state: { type: "object" } },
  });
  try {
    const spec = createRichOutputToolSchema().properties.spec;
    expect(spec.additionalProperties).toBe(false);
    expect(spec.properties).not.toHaveProperty("state");
  } finally {
    spy.mockRestore();
  }
});
