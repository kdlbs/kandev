import { describe, it, expect } from "vitest";
import { EXTERNAL_MCP_TOOL_GROUPS, countExternalMcpTools } from "./external-mcp-tools";
import enSettings from "@/src/locales/en/settings.json";

describe("external MCP tool catalog", () => {
  // NOTE: this pins the CATALOG, which currently lags the backend. `ModeExternal`
  // registers 33 tools (12 workflow + 4 agent + 4 mcp + 5 executor + 7 task +
  // 1 create_task — see `TestServerModeExternal_ToolCount`), and this catalog
  // lists 30: `list_repositories_kandev`, `import_workflow_kandev` and
  // `get_task_conversation_kandev` are missing, so the page under-advertises
  // the endpoint by three.
  //
  // That drift predates this file's localization and is left alone here on
  // purpose: adding the three means writing three new user-facing descriptions
  // that have to be checked against the backend's own wording, which is a
  // content change and not a copy migration. Update this number with them.
  it("lists 30 tools, the catalog's current contents", () => {
    expect(countExternalMcpTools()).toBe(30);
  });

  it("every tool name is unique and ends with the kandev suffix", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(new Set(names).size).toBe(names.length);
    for (const name of names) {
      expect(name.endsWith("_kandev")).toBe(true);
    }
  });

  it("includes create_task_kandev (the only task-spawning tool exposed externally)", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(names).toContain("create_task_kandev");
  });

  it("does not expose session-scoped tools (plan, ask_user_question)", () => {
    const names = EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools.map((t) => t.name));
    expect(names).not.toContain("ask_user_question_kandev");
    expect(names).not.toContain("create_task_plan_kandev");
  });

  it("every group has at least one tool and a non-empty title key", () => {
    for (const group of EXTERNAL_MCP_TOOL_GROUPS) {
      expect(group.titleKey.length).toBeGreaterThan(0);
      expect(group.tools.length).toBeGreaterThan(0);
    }
  });

  // The catalog is a plain `.ts` module with no JSX, so `i18next/no-literal-string`
  // never inspects it and a key that never made it into the catalog would render
  // as the raw `settings:externalMcp…` string with nothing failing.
  it("every title and description key resolves in the en catalog", () => {
    const catalog = enSettings as Record<string, string>;
    const keys = EXTERNAL_MCP_TOOL_GROUPS.flatMap((group) => [
      group.titleKey,
      group.descriptionKey,
      ...group.tools.map((tool) => tool.descriptionKey),
    ]);
    // Derived, not a literal: this asserts every entry carries keys, and must
    // not quietly re-pin the tool count the test above owns.
    expect(keys).toHaveLength(EXTERNAL_MCP_TOOL_GROUPS.length * 2 + countExternalMcpTools());
    for (const key of keys) {
      const [ns, name] = key.split(":");
      expect(ns).toBe("settings");
      expect(catalog[name], `missing catalog entry for ${key}`).toBeTruthy();
    }
  });

  // A message that interpolates a wire token renders a dead `{{placeholder}}`
  // if the tool forgets to carry the value.
  it("supplies a value for every interpolated description", () => {
    const catalog = enSettings as Record<string, string>;
    for (const tool of EXTERNAL_MCP_TOOL_GROUPS.flatMap((g) => g.tools)) {
      const message = catalog[tool.descriptionKey.split(":")[1]];
      const placeholders = [...message.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]);
      expect(Object.keys(tool.descriptionValues ?? {}).sort()).toEqual(placeholders.sort());
    }
  });
});
