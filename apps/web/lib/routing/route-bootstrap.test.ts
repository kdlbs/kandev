import { beforeEach, describe, expect, it } from "vitest";

import {
  mapWorkspaceItem,
  readActiveWorkspaceCookie,
  readCookie,
  resolveSettingsActiveWorkspaceId,
} from "./route-bootstrap";
import type { ListWorkspacesResponse } from "@/lib/types/http";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

beforeEach(() => {
  document.cookie = "kandev-active-workspace=; path=/; max-age=0";
  document.cookie = "office-active-workspace=; path=/; max-age=0";
});

describe("mapWorkspaceItem", () => {
  it("normalizes optional workspace fields for store hydration", () => {
    expect(
      mapWorkspaceItem({
        id: "ws-1",
        name: "Workspace",
        owner_id: "owner-1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-02T00:00:00Z",
      } as WorkspaceItem),
    ).toEqual({
      id: "ws-1",
      name: "Workspace",
      description: null,
      owner_id: "owner-1",
      default_executor_id: null,
      default_environment_id: null,
      default_agent_profile_id: null,
      default_config_agent_profile_id: null,
      office_workflow_id: null,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    });
  });
});

describe("readCookie", () => {
  it("reads encoded cookie values by encoded cookie name", () => {
    document.cookie = `${encodeURIComponent("office-active-workspace")}=${encodeURIComponent("ws 1/2")}`;

    expect(readCookie("office-active-workspace")).toBe("ws 1/2");
    expect(readCookie("missing")).toBeNull();
  });

  it("prefers the general active workspace cookie over the legacy office cookie", () => {
    document.cookie = "office-active-workspace=office-1; path=/";
    document.cookie = "kandev-active-workspace=kanban-1; path=/";

    expect(readActiveWorkspaceCookie()).toBe("kanban-1");
  });

  it("does not use the legacy office cookie as the generic active workspace", () => {
    document.cookie = "office-active-workspace=office-1; path=/";

    expect(readActiveWorkspaceCookie()).toBeNull();
  });
});

describe("resolveSettingsActiveWorkspaceId", () => {
  const officeWorkflow = "office-workflow";
  const officeWorkflow1 = "office-workflow-1";
  const officeWorkflow2 = "office-workflow-2";
  const kanbanOne = "kanban-1";
  const kanbanTwo = "kanban-2";

  it("falls back to a kanban workspace before an office workspace", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: "office-1", office_workflow_id: officeWorkflow },
          { id: kanbanOne, office_workflow_id: null },
        ],
        null,
        null,
        null,
      ),
    ).toBe(kanbanOne);
  });

  it("prefers explicit route workspace ID", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: "office-1", office_workflow_id: officeWorkflow },
          { id: kanbanOne, office_workflow_id: null },
        ],
        "office-1",
        null,
        null,
      ),
    ).toBe("office-1");
  });

  it("uses active cookie when it matches a kanban workspace", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: "office-1", office_workflow_id: officeWorkflow },
          { id: kanbanOne, office_workflow_id: null },
          { id: kanbanTwo, office_workflow_id: null },
        ],
        null,
        kanbanTwo,
        null,
      ),
    ).toBe(kanbanTwo);
  });

  it("falls back to settings workspace when no cookie matches", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: "office-1", office_workflow_id: officeWorkflow },
          { id: kanbanOne, office_workflow_id: null },
          { id: kanbanTwo, office_workflow_id: null },
        ],
        null,
        "ws-missing",
        kanbanOne,
      ),
    ).toBe(kanbanOne);
  });

  it("returns null when no workspaces exist", () => {
    expect(resolveSettingsActiveWorkspaceId([], null, "k-1", "k-2")).toBeNull();
  });

  it("falls back to office workspace only when no kanban workspaces exist", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: "office-1", office_workflow_id: officeWorkflow1 },
          { id: "office-2", office_workflow_id: officeWorkflow2 },
        ],
        null,
        "missing",
        null,
      ),
    ).toBe("office-1");
  });

  it("returns first kanban workspace when multiple kanban options exist", () => {
    expect(
      resolveSettingsActiveWorkspaceId(
        [
          { id: kanbanOne, office_workflow_id: null },
          { id: kanbanTwo, office_workflow_id: null },
        ],
        null,
        null,
        null,
      ),
    ).toBe(kanbanOne);
  });
});
