import { describe, expect, it } from "vitest";
import {
  marketplaceInputsComplete,
  mcpValueRecord,
  mcpValueRows,
  validMCPInputNames,
} from "./mcp-config-inputs";
import type { MCPMarketplaceChoice } from "@/lib/types/http-mcp";

describe("MCP setup inputs", () => {
  it("preserves values exactly and excludes values replaced by a secret reference", () => {
    const values = { REGION: " eu west ", API_KEY: "must not persist", EMPTY: "" };
    expect(
      mcpValueRecord(mcpValueRows(values), [{ input_name: "API_KEY", secret_id: "secret" }]),
    ).toEqual({ REGION: " eu west ", EMPTY: "" });
    expect(values.API_KEY).toBe("must not persist");
  });

  it("rejects blank and duplicated names without rejecting empty collections", () => {
    expect(validMCPInputNames([])).toBe(true);
    for (const names of [[""], [" KEY"], ["KEY", "KEY"]])
      expect(validMCPInputNames(names)).toBe(false);
  });

  it("requires secret references and non-secret values for publisher-required inputs", () => {
    const choice: MCPMarketplaceChoice = {
      id: "remote",
      kind: "remote",
      selectable: true,
      headers: [
        { name: "Authorization", isRequired: true, isSecret: true },
        { name: "X-Region", isRequired: true },
      ],
    };
    expect(
      marketplaceInputsComplete(choice, { Authorization: "literal", "X-Region": "eu" }, []),
    ).toBe(false);
    expect(
      marketplaceInputsComplete(choice, {}, [{ input_name: "Authorization", secret_id: "secret" }]),
    ).toBe(false);
    expect(
      marketplaceInputsComplete(choice, { "X-Region": "eu" }, [
        { input_name: "Authorization", secret_id: "secret" },
      ]),
    ).toBe(true);
  });
});
