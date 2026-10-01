"use client";

import { useTranslation } from "react-i18next";
import type { ControlAction } from "@/lib/api/domains/coordinator-api";
import type { ProjectsDraft, WatchesDraft } from "@/lib/coordinators/control-draft";
import type { GoalFormState } from "@/lib/coordinators/goal-form";
import {
  actionOfPath,
  actionPath,
  PROJECTS_PATHS,
  WATCHES_PATHS,
  type SetupErrors,
  type SetupState,
} from "@/lib/coordinators/setup";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import type { useWorkspaceBoards } from "@/hooks/domains/coordinator/use-workspace-boards";
import { CoordinatorFormFields } from "../coordinator-form-fields";
import { GoalFormFields } from "../sections/goal-form-fields";
import { MayDoRows } from "../sections/may-do-section";
import { ProjectsFields } from "../sections/projects-fields";
import { WatchesFields } from "../sections/watches-section";
import type { useWorkspaceProjects } from "@/hooks/domains/coordinator/use-workspace-projects";

type Edit = (patch: Partial<SetupState>, paths: readonly string[]) => void;

type BodyProps = {
  state: SetupState;
  messages: SetupErrors;
  edit: Edit;
  leave: (path: string) => void;
};

function firstMessage(messages: SetupErrors, matches: (path: string) => boolean): string | null {
  const path = Object.keys(messages).find(matches);
  return path === undefined ? null : messages[path];
}

function StepLine({ messages }: { messages: SetupErrors }) {
  const { t } = useTranslation();
  if (!messages[""]) return null;
  return (
    <p role="alert" className="text-sm text-destructive" data-testid="setup-step-error">
      {t(messages[""])}
    </p>
  );
}

type IdentityProps = BodyProps & {
  agentProfiles: readonly AgentProfileOption[];
  executors: readonly Executor[];
};

export function IdentityStep({
  state,
  messages,
  edit,
  leave,
  agentProfiles,
  executors,
}: IdentityProps) {
  const { t } = useTranslation();
  const translated = (path: string) => (messages[path] ? t(messages[path]) : undefined);
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <CoordinatorFormFields
        only={["name", "agent", "executor"]}
        form={{
          name: state.name,
          agentProfileId: state.agentProfileId,
          executorProfileId: state.executorProfileId,
          context: "",
        }}
        onChange={(key, value) => {
          if (key === "name") edit({ name: value }, ["name"]);
          if (key === "agentProfileId") edit({ agentProfileId: value }, ["agent_profile_id"]);
          if (key === "executorProfileId")
            edit({ executorProfileId: value }, ["executor_profile_id"]);
        }}
        onLeave={leave}
        disabled={false}
        agentProfiles={agentProfiles}
        executors={executors}
        fieldError={null}
        messages={{
          name: translated("name"),
          agent_profile_id: translated("agent_profile_id"),
          executor_profile_id: translated("executor_profile_id"),
        }}
      />
    </div>
  );
}

type WatchesProps = BodyProps & { boards: ReturnType<typeof useWorkspaceBoards> };

export function WatchesStep({ state, messages, edit, boards }: WatchesProps) {
  const { t } = useTranslation();
  const line = firstMessage(messages, (p) => p.startsWith("watches"));
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <WatchesFields
        watches={state.watches}
        boards={boards.boards}
        boardsStatus={boards.status}
        onBoardsRetry={boards.retry}
        canManage
        onChange={(next: WatchesDraft) => edit({ watches: next }, WATCHES_PATHS)}
        errorMessage={line ? t(line) : null}
      />
    </div>
  );
}

type ProjectsProps = BodyProps & { projects: ReturnType<typeof useWorkspaceProjects> };

export function ProjectsStep({ state, messages, edit, projects }: ProjectsProps) {
  const { t } = useTranslation();
  const line = firstMessage(messages, (p) => p.startsWith("projects"));
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <ProjectsFields
        projects={state.projects}
        sets={projects.sets}
        loose={projects.loose}
        status={projects.status}
        onRetry={projects.retry}
        canManage
        onChange={(next: ProjectsDraft) => edit({ projects: next }, PROJECTS_PATHS)}
        errorMessage={line ? t(line) : null}
      />
    </div>
  );
}

function goalMessages(messages: SetupErrors, t: (key: string) => string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [path, key] of Object.entries(messages)) {
    if (
      path === "goal.name" ||
      path === "goal.due_on" ||
      /^goal\.criteria\[\d+\]\.text$/.test(path)
    ) {
      out[path.slice("goal.".length)] = t(key);
    }
  }
  return out;
}

export function GoalStep({ state, messages, edit }: BodyProps) {
  const { t } = useTranslation();
  const line = firstMessage(
    messages,
    (p) => p === "goal.criteria" || /^goal\.criteria\[\d+\]\.id$/.test(p),
  );
  const touchGoal = (goal: GoalFormState) =>
    edit({ goal }, [
      "goal.name",
      "goal.due_on",
      "goal.criteria",
      ...goal.criteria.flatMap((c, i) =>
        c.text !== state.goal.criteria[i]?.text && state.goal.criteria[i]
          ? [`goal.criteria[${i}].text`]
          : [],
      ),
    ]);
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <GoalFormFields
        draft
        form={state.goal}
        disabled={false}
        fieldError={null}
        onChange={touchGoal}
        messages={goalMessages(messages, t)}
        criteriaMessage={line ? t(line) : null}
      />
    </div>
  );
}

export function ContextStep({
  state,
  messages,
  edit,
  leave,
  agentProfiles,
  executors,
}: IdentityProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <CoordinatorFormFields
        only={["context"]}
        form={{ name: "", agentProfileId: "", executorProfileId: "", context: state.context }}
        onChange={(_key, value) => edit({ context: value as string }, ["context"])}
        onLeave={() => leave("context")}
        disabled={false}
        agentProfiles={agentProfiles}
        executors={executors}
        fieldError={null}
        messages={{ context: messages.context ? t(messages.context) : undefined }}
      />
    </div>
  );
}

export function MayDoStep({ state, messages, edit }: BodyProps) {
  const { t } = useTranslation();
  const rowErrors: Partial<Record<ControlAction, string>> = {};
  for (const [path, key] of Object.entries(messages)) {
    const action = actionOfPath(path);
    if (action) rowErrors[action] = t(key);
  }
  const unknownRow = firstMessage(
    messages,
    (p) => p.startsWith("policy.actions.") && actionOfPath(p) === null,
  );
  const line = messages.policy ?? unknownRow;
  return (
    <div className="space-y-4">
      <StepLine messages={messages} />
      <MayDoRows
        actions={state.actions}
        canManage
        onChange={(action, value) =>
          edit({ actions: { ...state.actions, [action]: value } }, [actionPath(action)])
        }
        rowErrors={rowErrors}
        errorMessage={line ? t(line) : null}
      />
    </div>
  );
}
