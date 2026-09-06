import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { WorkspaceRepositoriesClient } from "@/app/settings/workspace/workspace-repositories-client";
import { WorkspaceWorkflowsClient } from "@/app/settings/workspace/workspace-workflows-client";
import Link from "@/components/routing/app-link";
import { IMPROVE_KANDEV_WORKSPACE_NAME } from "@/components/improve-kandev-dialog-model";
import { WorkflowEditorPage } from "@/components/settings/workflow-editor/workflow-editor-page";
import {
  DEFAULT_CUSTOM_STEPS,
  createDraftWorkflowSteps,
} from "@/app/settings/workspace/use-workflow-creation";
import { fetchJson } from "@/lib/api/client";
import { useSearchParams } from "@/lib/routing/client-router";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listWorkflowSteps, listWorkflowTemplates } from "@/lib/api/domains/workflow-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { generateUUID } from "@/lib/utils";
import type {
  Repository,
  RepositoryScript,
  Workflow,
  WorkflowTemplate,
  WorkflowStep,
  Workspace,
} from "@/lib/types/http";
import { workflowId as toWorkflowId, workspaceId as toWorkspaceId } from "@/lib/types/http";

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

type WorkspaceWorkflowEditorRouteState = {
  workspace: Workspace | null;
  workflow: Workflow | null;
  steps: WorkflowStep[];
  workflowTemplate?: WorkflowTemplate;
  initialName?: string;
};

export function WorkspaceWorkflowEditorRoute({
  workspaceId,
  workflowId,
}: {
  workspaceId: string;
  workflowId: string;
}): ReactNode {
  const [state, setState] = useState<WorkspaceWorkflowEditorRouteState | null>(null);
  const isNewWorkflow = workflowId === "new";
  const { t } = useTranslation();
  const searchParams = useSearchParams();
  const templateId = isNewWorkflow ? searchParams.get("template") : null;
  const initialName = isNewWorkflow ? (searchParams.get("name") ?? "") : "";

  useEffect(() => {
    let cancelled = false;
    setState(null);
    loadWorkspaceWorkflowEditorRoute(workspaceId, workflowId, templateId, initialName)
      .catch(() => ({ workspace: null, workflow: null, steps: [] }))
      .then((nextState) => {
        if (!cancelled) setState(nextState);
      });
    return () => {
      cancelled = true;
    };
  }, [initialName, templateId, workflowId, workspaceId]);

  if (!state?.workspace) return null;
  if (!isNewWorkflow && state.workflow === null) {
    return (
      <div className="space-y-3 p-6" data-testid="workflow-editor-not-found">
        <h2 className="text-lg font-semibold">{t("workflows:workflowNotFound")}</h2>
        <p className="text-sm text-muted-foreground">
          {t("workflows:workflowNotFoundDescription")}
        </p>
        <Link href={`/settings/workspaces/${workspaceId}/workflows`}>
          {t("workflows:backToWorkflows")}
        </Link>
      </div>
    );
  }
  const workflow =
    state.workflow ?? createNewWorkflow(state.workspace, state.workflowTemplate, state.initialName);
  const definitions = state.workflowTemplate?.default_steps?.length
    ? state.workflowTemplate.default_steps
    : DEFAULT_CUSTOM_STEPS;
  const steps = state.workflow ? state.steps : createDraftWorkflowSteps(workflow.id, definitions);
  return (
    <WorkflowEditorPage
      workspace={state.workspace}
      workflow={workflow}
      steps={steps}
      isNewWorkflow={isNewWorkflow}
    />
  );
}

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
    <WorkspaceRepositoriesClient
      workspace={state.workspace}
      repositories={state.repositories}
      isImproveWorkspace={state.workspace?.name === IMPROVE_KANDEV_WORKSPACE_NAME}
    />
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
      isImproveWorkspace={state.workspace?.name === IMPROVE_KANDEV_WORKSPACE_NAME}
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

async function loadWorkspaceWorkflowEditorRoute(
  workspaceId: string,
  workflowId: string,
  templateId: string | null,
  initialName: string,
): Promise<WorkspaceWorkflowEditorRouteState> {
  const workspace = await fetchJson<Workspace>(`/api/v1/workspaces/${workspaceId}`, {
    cache: "no-store",
  });
  if (workflowId === "new") {
    const templateResponse = templateId
      ? await listWorkflowTemplates({ cache: "no-store" })
      : undefined;
    return {
      workspace,
      workflow: null,
      steps: [],
      workflowTemplate: templateResponse?.templates?.find((item) => item.id === templateId),
      initialName,
    };
  }
  const workflowResponse = await listWorkflows(workspaceId, { cache: "no-store" });
  const workflow = workflowResponse.workflows?.find((item) => item.id === workflowId) ?? null;
  if (!workflow) return { workspace, workflow: null, steps: [] };
  const stepResponse = await listWorkflowSteps(workflow.id, { cache: "no-store" });
  return { workspace, workflow, steps: stepResponse.steps ?? [] };
}

function createNewWorkflow(
  workspace: Workspace,
  template?: WorkflowTemplate,
  initialName = "",
): Workflow {
  const clientId = `temp-workflow-${generateUUID()}`;
  // i18n-exempt: persisted workflow name fallback, not rendered copy.
  const name = initialName.trim() || template?.name || "New Workflow";
  return {
    id: toWorkflowId(clientId),
    workspace_id: toWorkspaceId(workspace.id),
    name,
    description: template?.description,
    workflow_template_id: template?.id,
    created_at: "",
    updated_at: "",
  };
}
