import { expect, it } from "vitest";
import cases from "../../../backend/internal/mcp/server/testdata/rich_output_composition_cases.json";
import { validateComposition } from "./validate";
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2, AC-AGENTS-AGENT-RICH-OUTPUT-002.3
it.each(cases)("agrees with the shared contract: $name", ({ input, valid }) => {
  expect(validateComposition(input) !== null).toBe(valid);
});
