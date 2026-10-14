import { describe, expect, it } from "vitest";
import { buildCursorMcpSelectionGroups } from "./cursor-mcp-selection";
import type { AgentMcpDiscoveryResponse } from "@/lib/types/agent-mcp-discovery";

const REMOVED_SERVER_ID = "removed-server";

const discovery: AgentMcpDiscoveryResponse = {
  agent_id: "agent-1",
  provider_id: "cursor",
  status: "ready",
  servers: [
    {
      id: "plugin-atlassian-jira",
      name: "Jira",
      plugin_name: "Atlassian",
      source_kind: "plugin",
      credentials_available: true,
    },
    {
      id: "plugin-atlassian-confluence",
      name: "Confluence",
      plugin_name: "Atlassian",
      source_kind: "plugin",
      credentials_available: false,
    },
    {
      id: "filesystem",
      name: "Filesystem",
      source_kind: "global",
      credentials_available: false,
    },
  ],
};

describe("buildCursorMcpSelectionGroups grouped results", () => {
  it("groups discovered servers by plugin and preserves saved unavailable IDs", () => {
    const groups = buildCursorMcpSelectionGroups(discovery, [
      "plugin-atlassian-jira",
      REMOVED_SERVER_ID,
    ]);

    expect(groups).toEqual([
      {
        id: "plugin:Atlassian",
        pluginName: "Atlassian",
        servers: [
          {
            id: "plugin-atlassian-jira",
            name: "Jira",
            pluginName: "Atlassian",
            sourceKind: "plugin",
            credentialsAvailable: true,
            selected: true,
            unavailable: false,
          },
          {
            id: "plugin-atlassian-confluence",
            name: "Confluence",
            pluginName: "Atlassian",
            sourceKind: "plugin",
            credentialsAvailable: false,
            selected: false,
            unavailable: false,
          },
        ],
      },
      {
        id: "source:global",
        pluginName: null,
        servers: [
          {
            id: "filesystem",
            name: "Filesystem",
            pluginName: null,
            sourceKind: "global",
            credentialsAvailable: false,
            selected: false,
            unavailable: false,
          },
        ],
      },
      {
        id: "unavailable-selections",
        pluginName: null,
        servers: [
          {
            id: REMOVED_SERVER_ID,
            name: REMOVED_SERVER_ID,
            pluginName: null,
            sourceKind: "unknown",
            credentialsAvailable: false,
            selected: true,
            unavailable: true,
          },
        ],
      },
    ]);
  });
});

describe("buildCursorMcpSelectionGroups unavailable state", () => {
  it("does not invent unavailable rows when discovery is not ready", () => {
    expect(
      buildCursorMcpSelectionGroups({ ...discovery, status: "unavailable", servers: [] }, [
        "saved-server",
      ]),
    ).toEqual([
      {
        id: "unavailable-selections",
        pluginName: null,
        servers: [
          {
            id: "saved-server",
            name: "saved-server",
            pluginName: null,
            sourceKind: "unknown",
            credentialsAvailable: false,
            selected: true,
            unavailable: true,
          },
        ],
      },
    ]);
  });

  it("keeps plugin names distinct from internal fallback groups", () => {
    const groups = buildCursorMcpSelectionGroups(
      {
        ...discovery,
        servers: [
          {
            id: "plugin-other",
            name: "Plugin server",
            plugin_name: "Other servers",
            source_kind: "plugin",
            credentials_available: true,
          },
        ],
      },
      [REMOVED_SERVER_ID],
    );

    expect(groups.map((group) => group.id)).toEqual([
      "plugin:Other servers",
      "unavailable-selections",
    ]);
  });
});
