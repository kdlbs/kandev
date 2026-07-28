import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  listSentryInstances,
  listSentryOrganizations,
  listSentryProjects,
} from "@/lib/api/domains/sentry-api";
import type { SentryConfig, SentryIssueWatch } from "@/lib/types/sentry";
import type * as SentryIssueWatchMultiselectModule from "./sentry-issue-watch-multiselect";
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
vi.mock("./sentry-issue-watch-multiselect", async (importOriginal) => {
  const actual = await importOriginal<typeof SentryIssueWatchMultiselectModule>();
  return {
    ...actual,
    LevelMultiSelect: () => null,
    StatusMultiSelect: () => null,
  };
});
vi.mock("./sentry-issue-watch-throttle-field", () => ({ MaxInflightTasksField: () => null }));
vi.mock("@/lib/api/domains/sentry-api", () => ({
  listSentryInstances: vi.fn(),
  listSentryOrganizations: vi.fn(),
  listSentryProjects: vi.fn(),
}));

const PRIMARY_INSTANCE_ID = "instance-a";
const FRONTEND_OPTION = "Frontend (frontend)";
const SENTRY_INSTANCE_FIELD = "Sentry instance";

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
    filter: { orgSlug: "acme", projectSlugs: ["frontend"] },
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
        name: instanceId === PRIMARY_INSTANCE_ID ? "Frontend" : "Backend",
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

    await choose(SENTRY_INSTANCE_FIELD, "Production");
    await waitFor(() => {
      expect(listSentryOrganizations).toHaveBeenCalledWith(WORKSPACE_ID, PRIMARY_INSTANCE_ID);
      expect(listSentryProjects).toHaveBeenCalledWith(WORKSPACE_ID, PRIMARY_INSTANCE_ID);
    });

    await choose(SENTRY_INSTANCE_FIELD, "Self-hosted");

    fireEvent.click(selectTrigger("Organization slug"));
    expect(screen.queryByRole("option", { name: "acme" })).toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });

    fireEvent.click(selectTrigger("Project slug"));
    expect(screen.queryByRole("option", { name: FRONTEND_OPTION })).toBeNull();
  });

  it("allows selecting multiple projects for a single Sentry watcher", async () => {
    vi.mocked(listSentryProjects).mockResolvedValue({
      projects: [
        { id: "frontend", slug: "frontend", name: "Frontend", orgSlug: "acme" },
        { id: "backend", slug: "backend", name: "Backend", orgSlug: "acme" },
      ],
    });

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
    await choose(SENTRY_INSTANCE_FIELD, "Production");
    await waitFor(() => {
      expect(listSentryProjects).toHaveBeenCalledWith(WORKSPACE_ID, PRIMARY_INSTANCE_ID);
    });

    fireEvent.click(selectTrigger("Project slug"));
    fireEvent.click(await screen.findByRole("option", { name: FRONTEND_OPTION }));
    fireEvent.click(await screen.findByRole("option", { name: "Backend (backend)" }));

    expect(screen.getByTestId("sentry-watch-project-trigger").textContent).toContain(
      "2 projects selected",
    );

    // Deselecting one falls back to showing the sole remaining project's name.
    fireEvent.click(screen.getByRole("option", { name: FRONTEND_OPTION }));
    await waitFor(() => {
      expect(screen.getByTestId("sentry-watch-project-trigger").textContent).toContain(
        "Backend (backend)",
      );
    });
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
    expect(selectTrigger(SENTRY_INSTANCE_FIELD).disabled).toBe(true);
  });
});
