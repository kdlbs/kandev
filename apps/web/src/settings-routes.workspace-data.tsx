import { useEffect, useState, type ReactNode } from "react";

import { WorkspaceRepositoriesClient } from "@/app/settings/workspace/workspace-repositories-client";
import { WorkspaceWorkflowsClient } from "@/app/settings/workspace/workspace-workflows-client";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listWorkflowTemplates } from "@/lib/api/domains/workflow-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import type {
  Repository,
  RepositoryScript,
  Workflow,
  WorkflowTemplate,
  Workspace,
} from "@/lib/types/http";

/**
 * The two workspace settings tabs that fetch their own data instead of reading
 * the hydrated store: Repositories and Workflows. Both existed as page-level
 * loaders under the Next.js runtime, so the SPA route table has to do the
 * fetching the removed server components used to. They live here rather than in
 * `settings-routes.tsx` to keep that file inside its line budget.
 */

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type WorkspaceRepositoriesRouteState = {
  workspace: Workspace | null;
  repositories: RepositoryWithScripts[];
};

type WorkspaceWorkflowsRouteState = {
  workspace: Workspace | null;
  workflows: Workflow[];
  workflowTemplates: WorkflowTemplate[];
};

export function WorkspaceRepositoriesRoute({ workspaceId }: { workspaceId: string }): ReactNode {
  const [state, setState] = useState<WorkspaceRepositoriesRouteState | null>(null);

  useEffect(() => {
    let cancelled = false;
    setState(null);

    loadWorkspaceRepositoriesRoute(workspaceId)
      .catch(() => ({ workspace: null, repositories: [] }))
      .then((nextState) => {
        if (!cancelled) setState(nextState);
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  if (!state) return null;
  return (
    <WorkspaceRepositoriesClient workspace={state.workspace} repositories={state.repositories} />
  );
}

export function WorkspaceWorkflowsRoute({ workspaceId }: { workspaceId: string }): ReactNode {
  const [state, setState] = useState<WorkspaceWorkflowsRouteState | null>(null);

  useEffect(() => {
    let cancelled = false;
    setState(null);

    loadWorkspaceWorkflowsRoute(workspaceId)
      .catch(() => ({ workspace: null, workflows: [], workflowTemplates: [] }))
      .then((nextState) => {
        if (!cancelled) setState(nextState);
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  if (!state) return null;
  return (
    <WorkspaceWorkflowsClient
      workspace={state.workspace}
      workflows={state.workflows}
      workflowTemplates={state.workflowTemplates}
    />
  );
}

async function loadWorkspaceRepositoriesRoute(
  workspaceId: string,
): Promise<WorkspaceRepositoriesRouteState> {
  const [workspace, repoResponse] = await Promise.all([
    fetchJson<Workspace>(`/api/v1/workspaces/${workspaceId}`, { cache: "no-store" }),
    listRepositories(workspaceId, { includeScripts: true }, { cache: "no-store" }),
  ]);

  return {
    workspace,
    repositories: repoResponse.repositories.map((repository) => ({
      ...repository,
      scripts: repository.scripts ?? [],
    })),
  };
}

async function loadWorkspaceWorkflowsRoute(
  workspaceId: string,
): Promise<WorkspaceWorkflowsRouteState> {
  const [workspace, workflowResponse, templateResponse] = await Promise.all([
    fetchJson<Workspace>(`/api/v1/workspaces/${workspaceId}`, { cache: "no-store" }),
    listWorkflows(workspaceId, { cache: "no-store" }),
    listWorkflowTemplates({ cache: "no-store" }),
  ]);

  return {
    workspace,
    workflows: workflowResponse.workflows ?? [],
    workflowTemplates: templateResponse.templates ?? [],
  };
}
