import type { TaskOrigin, TaskPriority, TaskState } from "@/lib/types/http";
import type {
  SidebarTaskColorAutomation,
  SidebarTaskColorDimension,
  SidebarTaskColorRule,
} from "@/lib/task-color-automation-settings";
import {
  fixedAutomaticTaskColor,
  parseWorkflowStepColor,
  type TaskMarkerPresentation,
} from "@/lib/task-color-presentation";
import {
  isRepositoryRuleTarget,
  repositoryRuleTargetMatches,
  type TaskRepositoryRuleIdentity,
} from "./repository-rule-identity";

export type TaskColorFacts = {
  workspaceId?: string;
  workflowId?: string;
  workflowStepId?: string;
  workflowStepColor?: string;
  state?: TaskState | string;
  priority?: TaskPriority | string;
  origin?: TaskOrigin | string;
  primaryExecutorProfileId?: string;
  repositories: readonly TaskRepositoryRuleIdentity[];
};

export type AutomaticTaskColorSource = {
  ruleId: string;
  dimension: SidebarTaskColorDimension;
  label: string;
  outputKind: "fixed" | "workflow_step";
  color?: string;
};

export type AutomaticTaskColorResult = {
  color: TaskMarkerPresentation;
  source: AutomaticTaskColorSource;
};

export function resolveAutomaticTaskColor(
  settings: SidebarTaskColorAutomation,
  facts: TaskColorFacts,
): AutomaticTaskColorResult | null {
  if (!settings.enabled) return null;
  for (const rule of settings.rules) {
    if (!rule.enabled || rule.condition.value === null || rule.condition.value === undefined) {
      continue;
    }
    if (!matchesRule(rule, facts)) continue;
    const color =
      rule.output.kind === "workflow_step"
        ? parseWorkflowStepColor(facts.workflowStepColor)
        : fixedAutomaticTaskColor(rule.output.color);
    return {
      color,
      source: {
        ruleId: rule.id,
        dimension: rule.condition.dimension,
        label: rule.condition.label,
        outputKind: rule.output.kind,
        color: rule.output.kind === "fixed" ? rule.output.color : undefined,
      },
    };
  }
  return null;
}

function matchesRule(rule: SidebarTaskColorRule, facts: TaskColorFacts): boolean {
  return RULE_MATCHERS[rule.condition.dimension](rule.condition.value, facts);
}

type RuleMatcher = (value: unknown, facts: TaskColorFacts) => boolean;

const RULE_MATCHERS: Record<SidebarTaskColorDimension, RuleMatcher> = {
  workflow_step: (value, facts) =>
    matchesScopedTarget(value, facts, "step_id", facts.workflowStepId),
  workflow: (value, facts) => matchesScopedTarget(value, facts, "workflow_id", facts.workflowId),
  repository: (value, facts) =>
    isRepositoryRuleTarget(value) &&
    repositoryRuleTargetMatches(value, facts.workspaceId, facts.repositories),
  executor_profile: (value, facts) => matchesFact(value, facts.primaryExecutorProfileId),
  task_state: (value, facts) => matchesFact(value, facts.state),
  priority: (value, facts) => matchesFact(value, facts.priority),
  origin: (value, facts) => value === (facts.origin ?? "kanban"),
};

function matchesScopedTarget(
  value: unknown,
  facts: TaskColorFacts,
  field: "step_id" | "workflow_id",
  expected: string | undefined,
): boolean {
  if (!isRecord(value)) return false;
  return value.workspace_id === facts.workspaceId && value[field] === expected;
}

function matchesFact(value: unknown, expected: string | undefined): boolean {
  return expected !== undefined && value === expected;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
