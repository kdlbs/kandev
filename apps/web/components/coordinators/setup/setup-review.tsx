"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Button } from "@kandev/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { CONTROL_ACTIONS } from "@/lib/coordinators/control-draft";
import { formatCalendarDate } from "@/lib/coordinators/goal-form";
import { flattenExecutorProfiles } from "@/lib/coordinators/profile-lookup";
import {
  contextReviewValue,
  isGoalEmpty,
  type SetupState,
  type SetupStepId,
} from "@/lib/coordinators/setup";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import type { ProjectChoice } from "@/lib/coordinators/control-draft";
import type { WorkspaceBoard } from "@/hooks/domains/coordinator/use-workspace-boards";

const ACTION_LABEL = {
  create_task: "coordinator:mayDoCreateTask",
  start_agent: "coordinator:mayDoStartAgent",
  message: "coordinator:mayDoMessage",
  move: "coordinator:mayDoMove",
  resume: "coordinator:mayDoResume",
  stop: "coordinator:mayDoStop",
} as const;

const SETTING_LABEL = {
  denied: "coordinator:mayDoDenied",
  requires_approval: "coordinator:mayDoRequiresApproval",
  automatic: "coordinator:mayDoAutomatic",
} as const;

type Row = { id: string; setting: string; value: string; owner: string; step: SetupStepId };

type Props = {
  state: SetupState;
  agentProfiles: readonly AgentProfileOption[];
  executors: readonly Executor[];
  boards: readonly WorkspaceBoard[];
  /** The selectable projects; undefined while the Projects step is not offered. */
  projectChoices?: readonly ProjectChoice[];
  onChange: (step: SetupStepId) => void;
};

function projectsReviewValue(
  state: SetupState,
  choices: readonly ProjectChoice[] | undefined,
  t: (key: string) => string,
): string | null {
  if (!choices) return null;
  const { scope, entries, includeNoRepository } = state.projects;
  if (scope === "all") return t("coordinator:setupEveryProject");
  const chosen = new Set(entries.map((entry) => `${entry.kind}:${entry.id}`));
  const names = choices
    .filter((choice) => chosen.has(`${choice.kind}:${choice.id}`))
    .map((choice) => choice.name)
    .join(", ");
  if (!includeNoRepository) return names;
  if (names === "") return t("coordinator:watchesIncludeNoRepository");
  return `${names} ${t("coordinator:copilotProjectsNoRepository")}`;
}

function identityRows(
  state: SetupState,
  agentProfiles: Props["agentProfiles"],
  executors: Props["executors"],
  identity: string,
  t: TFunction,
): Row[] {
  const agent = agentProfiles.find((p) => p.id === state.agentProfileId)?.label;
  const profiles = flattenExecutorProfiles(executors);
  const executor = profiles.find((p) => p.id === state.executorProfileId)?.name;
  const taskAgent = agentProfiles.find((p) => p.id === state.taskAgentProfileId)?.label;
  const taskExecutor = profiles.find((p) => p.id === state.taskExecutorProfileId)?.name;
  return [
    {
      id: "name",
      setting: t("coordinator:nameLabel"),
      value: state.name.trim(),
      owner: identity,
      step: "identity",
    },
    {
      id: "agent",
      setting: t("coordinator:agentProfileLabel"),
      value: agent ?? state.agentProfileId,
      owner: identity,
      step: "identity",
    },
    {
      id: "executor",
      setting: t("coordinator:executorLabel"),
      value: executor ?? state.executorProfileId,
      owner: identity,
      step: "identity",
    },
    {
      id: "taskPair",
      setting: t("coordinator:taskPairLabel"),
      value: t("coordinator:setupTaskPairValue", {
        agent: taskAgent ?? state.taskAgentProfileId,
        executor: taskExecutor ?? state.taskExecutorProfileId,
      }),
      owner: identity,
      step: "identity",
    },
  ];
}

function useReviewRows({
  state,
  agentProfiles,
  executors,
  boards,
  projectChoices,
}: Omit<Props, "onChange">): Row[] {
  const { t, i18n } = useTranslation();
  const notSet = t("coordinator:setupNotSet");
  const selected = new Set(state.watches.workflowIds);
  const watches =
    state.watches.scope === "all"
      ? t("coordinator:setupEveryBoard")
      : boards
          .filter((b) => selected.has(b.id))
          .map((b) => b.name)
          .join(", ");
  const projects = projectsReviewValue(state, projectChoices, t);
  const goal = state.goal;
  const due =
    goal.dueOn === ""
      ? t("coordinator:setupGoalNoDue")
      : t("coordinator:setupGoalDue", { date: formatCalendarDate(goal.dueOn, i18n.language) });
  const goalValue = isGoalEmpty(goal)
    ? notSet
    : t("coordinator:setupGoalValue", { name: goal.name.trim(), due, count: goal.criteria.length });
  const identity = t("coordinator:sectionIdentity");
  const rows: Row[] = [
    ...identityRows(state, agentProfiles, executors, identity, t),
    {
      id: "watches",
      setting: t("coordinator:sectionWatches"),
      value: watches,
      owner: t("coordinator:sectionWatches"),
      step: "watches",
    },
    ...(projects === null
      ? []
      : [
          {
            id: "projects",
            setting: t("coordinator:watchesProjects"),
            value: projects,
            owner: t("coordinator:sectionWatches"),
            step: "projects" as const,
          },
        ]),
    {
      id: "goal",
      setting: t("coordinator:sectionGoal"),
      value: goalValue,
      owner: t("coordinator:sectionGoal"),
      step: "goal",
    },
    {
      id: "context",
      setting: t("coordinator:contextLabel"),
      value: contextReviewValue(state.context) ?? notSet,
      owner: identity,
      step: "context",
    },
  ];
  for (const action of CONTROL_ACTIONS) {
    rows.push({
      id: action,
      setting: t(ACTION_LABEL[action]),
      value: t(SETTING_LABEL[state.actions[action]]),
      owner: t("coordinator:sectionMayDo"),
      step: "may-do",
    });
  }
  return rows;
}

export function SetupReview(props: Props) {
  const { t } = useTranslation();
  const rows = useReviewRows(props);
  return (
    <div className="space-y-3" data-testid="setup-review">
      <h2 className="text-base font-semibold">{t("coordinator:setupReviewTitle")}</h2>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("coordinator:setupColSetting")}</TableHead>
            <TableHead>{t("coordinator:setupColValue")}</TableHead>
            <TableHead>{t("coordinator:setupColOwner")}</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.id} data-testid={`setup-review-row-${row.id}`}>
              <TableCell className="font-medium">{row.setting}</TableCell>
              <TableCell
                className="whitespace-normal break-words"
                data-testid={`setup-review-value-${row.id}`}
              >
                {row.value}
              </TableCell>
              <TableCell>{row.owner}</TableCell>
              <TableCell>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="cursor-pointer"
                  data-testid={`setup-review-change-${row.id}`}
                  aria-label={t("coordinator:setupChangeRow", { setting: row.setting })}
                  onClick={() => props.onChange(row.step)}
                >
                  {t("coordinator:setupChange")}
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
