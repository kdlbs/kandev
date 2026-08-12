import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { IMPROVE_KANDEV_WORKSPACE_NAME } from "@/components/improve-kandev-dialog-model";

/**
 * The Improve Kandev workspace is configuration-immutable, and the only thing
 * that switches its Repositories/Workflows tabs into read-only mode is the
 * `isImproveWorkspace` prop these route wrappers pass down. It was dropped once
 * already when the routes moved out of `settings-routes.tsx`, silently
 * re-enabling editing; these tests exist so that cannot happen again.
 */

const { repositoriesProps, workflowsProps } = vi.hoisted(() => ({
  repositoriesProps: [] as { isImproveWorkspace?: boolean }[],
  workflowsProps: [] as { isImproveWorkspace?: boolean }[],
}));

vi.mock("@/app/settings/workspace/workspace-repositories-client", () => ({
  WorkspaceRepositoriesClient: (props: { isImproveWorkspace?: boolean }) => {
    repositoriesProps.push(props);
    return <div data-testid="repositories-client">{String(props.isImproveWorkspace)}</div>;
  },
}));

vi.mock("@/app/settings/workspace/workspace-workflows-client", () => ({
  WorkspaceWorkflowsClient: (props: { isImproveWorkspace?: boolean }) => {
    workflowsProps.push(props);
    return <div data-testid="workflows-client">{String(props.isImproveWorkspace)}</div>;
  },
}));

const fetchJson = vi.fn();
const listRepositories = vi.fn();
const listWorkflows = vi.fn();
const listWorkflowTemplates = vi.fn();

vi.mock("@/lib/api/client", () => ({ fetchJson: (...args: unknown[]) => fetchJson(...args) }));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: (...args: unknown[]) => listRepositories(...args),
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: (...args: unknown[]) => listWorkflows(...args),
}));
vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowTemplates: (...args: unknown[]) => listWorkflowTemplates(...args),
}));

const { WorkspaceRepositoriesRoute, WorkspaceWorkflowsRoute } =
  await import("./settings-routes.workspace-data");

function mockWorkspace(name: string) {
  fetchJson.mockResolvedValue({ id: "ws-1", name });
  listRepositories.mockResolvedValue({ repositories: [] });
  listWorkflows.mockResolvedValue({ workflows: [] });
  listWorkflowTemplates.mockResolvedValue({ templates: [] });
}

beforeEach(() => {
  vi.clearAllMocks();
  repositoriesProps.length = 0;
  workflowsProps.length = 0;
});

describe("WorkspaceRepositoriesRoute", () => {
  it("marks the Improve Kandev workspace as read-only", async () => {
    mockWorkspace(IMPROVE_KANDEV_WORKSPACE_NAME);

    render(<WorkspaceRepositoriesRoute workspaceId="ws-1" />);

    await screen.findByTestId("repositories-client");
    expect(repositoriesProps.at(-1)?.isImproveWorkspace).toBe(true);
  });

  it("leaves a regular workspace editable", async () => {
    mockWorkspace("My Workspace");

    render(<WorkspaceRepositoriesRoute workspaceId="ws-1" />);

    await screen.findByTestId("repositories-client");
    expect(repositoriesProps.at(-1)?.isImproveWorkspace).toBe(false);
  });
});

describe("WorkspaceWorkflowsRoute", () => {
  it("marks the Improve Kandev workspace as read-only", async () => {
    mockWorkspace(IMPROVE_KANDEV_WORKSPACE_NAME);

    render(<WorkspaceWorkflowsRoute workspaceId="ws-1" />);

    await screen.findByTestId("workflows-client");
    expect(workflowsProps.at(-1)?.isImproveWorkspace).toBe(true);
  });

  it("leaves a regular workspace editable", async () => {
    mockWorkspace("My Workspace");

    render(<WorkspaceWorkflowsRoute workspaceId="ws-1" />);

    await screen.findByTestId("workflows-client");
    expect(workflowsProps.at(-1)?.isImproveWorkspace).toBe(false);
  });
});
