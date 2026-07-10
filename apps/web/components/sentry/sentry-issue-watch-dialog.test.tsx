import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  listSentryInstances,
  listSentryOrganizations,
  listSentryProjects,
} from "@/lib/api/domains/sentry-api";
import type { SentryConfig, SentryIssueWatch } from "@/lib/types/sentry";
import { SentryIssueWatchDialog } from "./sentry-issue-watch-dialog";

const { WORKSPACE_ID } = vi.hoisted(() => ({ WORKSPACE_ID: "ws-1" }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: WORKSPACE_ID, items: [{ id: WORKSPACE_ID, name: "Workspace" }] },
      workflows: { items: [] },
      agentProfiles: { items: [] },
      executors: { items: [] },
    }),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/use-workflows", () => ({ useWorkflows: vi.fn() }));
vi.mock("@/hooks/use-workflow-steps", () => ({
  useWorkflowSteps: () => ({ steps: [], loading: false }),
  stepPlaceholder: () => "Select workflow first",
}));
vi.mock("@/components/watcher-repository-fields", () => ({ WatcherRepositoryFields: () => null }));
vi.mock("@/components/settings/profile-edit/script-editor", () => ({
  ScriptEditor: () => null,
  computeEditorHeight: () => 0,
}));
vi.mock("./sentry-issue-watch-multiselect", () => ({
  LevelMultiSelect: () => null,
  StatusMultiSelect: () => null,
}));
vi.mock("./sentry-issue-watch-throttle-field", () => ({ MaxInflightTasksField: () => null }));
vi.mock("@/lib/api/domains/sentry-api", () => ({
  listSentryInstances: vi.fn(),
  listSentryOrganizations: vi.fn(),
  listSentryProjects: vi.fn(),
}));

const PRIMARY_INSTANCE_ID = "instance-a";

function sentryInstance(id: string, name: string): SentryConfig {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    name,
    authMethod: "auth_token",
    url: "https://sentry.io",
    hasSecret: true,
    lastOk: true,
    createdAt: "",
    updatedAt: "",
  };
}

function legacyUnboundWatch(): SentryIssueWatch {
  return {
    id: "watch-1",
    workspaceId: WORKSPACE_ID,
    sentryInstanceId: "",
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    repositoryId: "",
    baseBranch: "",
    filter: { orgSlug: "acme", projectSlug: "frontend" },
    agentProfileId: "",
    executorProfileId: "",
    prompt: "Handle the issue.",
    enabled: true,
    pollIntervalSeconds: 300,
    maxInflightTasks: 5,
    createdAt: "",
    updatedAt: "",
  };
}

function selectTrigger(label: string): HTMLButtonElement {
  const trigger = screen.getByText(label).parentElement?.querySelector("button");
  if (!(trigger instanceof HTMLButtonElement)) throw new Error(`Missing ${label} selector`);
  return trigger;
}

async function choose(label: string, option: string): Promise<void> {
  fireEvent.click(selectTrigger(label));
  fireEvent.click(await screen.findByRole("option", { name: option }));
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

beforeEach(() => {
  vi.mocked(listSentryInstances).mockResolvedValue([
    sentryInstance(PRIMARY_INSTANCE_ID, "Production"),
    sentryInstance("instance-b", "Self-hosted"),
  ]);
  vi.mocked(listSentryOrganizations).mockImplementation(async (_workspaceId, instanceId) => ({
    organizations: [
      { id: instanceId, slug: instanceId === PRIMARY_INSTANCE_ID ? "acme" : "globex", name: "" },
    ],
  }));
  vi.mocked(listSentryProjects).mockImplementation(async (_workspaceId, instanceId) => ({
    projects: [
      {
        id: instanceId,
        slug: instanceId === PRIMARY_INSTANCE_ID ? "frontend" : "backend",
        name: "",
        orgSlug: instanceId === PRIMARY_INSTANCE_ID ? "acme" : "globex",
      },
    ],
  }));
});

describe("SentryIssueWatchDialog", () => {
  it("removes the previous instance's org and project choices before the new lookup resolves", async () => {
    render(
      <SentryIssueWatchDialog
        open
        onOpenChange={vi.fn()}
        watch={null}
        workspaceId={WORKSPACE_ID}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(listSentryInstances).toHaveBeenCalledWith(WORKSPACE_ID);
    });

    await choose("Sentry instance", "Production");
    await waitFor(() => {
      expect(listSentryOrganizations).toHaveBeenCalledWith(WORKSPACE_ID, PRIMARY_INSTANCE_ID);
      expect(listSentryProjects).toHaveBeenCalledWith(WORKSPACE_ID, PRIMARY_INSTANCE_ID);
    });

    await choose("Sentry instance", "Self-hosted");

    fireEvent.click(selectTrigger("Organization slug"));
    expect(screen.queryByRole("option", { name: "acme" })).toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });

    fireEvent.click(selectTrigger("Project slug"));
    expect(screen.queryByRole("option", { name: "Frontend (frontend)" })).toBeNull();
  });

  it("permits mutable updates to a legacy unbound watch while its instance remains immutable", async () => {
    render(
      <SentryIssueWatchDialog
        open
        onOpenChange={vi.fn()}
        watch={legacyUnboundWatch()}
        workspaceId={WORKSPACE_ID}
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect((screen.getByRole("button", { name: "Update" }) as HTMLButtonElement).disabled).toBe(
        false,
      );
    });
    expect(selectTrigger("Sentry instance").disabled).toBe(true);
  });
});
