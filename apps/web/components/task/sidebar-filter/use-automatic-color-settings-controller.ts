"use client";

import { useEffect, useMemo, useState } from "react";
import { arrayMove } from "@dnd-kit/sortable";
import { useAppStore } from "@/components/state-provider";
import {
  isRepositoryRuleTarget,
  type RepositoryRuleTarget,
} from "@/lib/sidebar/repository-rule-identity";
import {
  MAX_AUTOMATIC_TASK_COLOR_RULES,
  type SidebarTaskColorRule,
} from "@/lib/task-color-automation-settings";
import { useRepositoryRuleCatalog } from "./use-repository-rule-catalog";
import { useSidebarTaskColorAutomation } from "./use-sidebar-task-color-automation";
import { useTaskColorRuleOptions } from "./task-color-rule-options";
import { createAutomaticColorRule } from "./automatic-color-rule-settings-helpers";

export function useAutomaticColorSettingsController() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const { value, update, saving, error } = useSidebarTaskColorAutomation();
  const scalarOptions = useTaskColorRuleOptions();
  const [expanded, setExpanded] = useState(false);
  const [activeRepositoryRuleId, setActiveRepositoryRuleId] = useState<string | null>(null);
  const [repositoryQuery, setRepositoryQuery] = useState("");
  const unavailableTargets = useMemo(() => findUnavailableTargets(value.rules), [value.rules]);
  const repositoryCatalog = useRepositoryRuleCatalog(workspaceId, expanded, unavailableTargets);

  useEffect(() => {
    if (activeRepositoryRuleId && !value.rules.some((rule) => rule.id === activeRepositoryRuleId)) {
      setActiveRepositoryRuleId(null);
    }
  }, [activeRepositoryRuleId, value.rules]);

  function updateSettings(nextRules: SidebarTaskColorRule[] | undefined, enabled = value.enabled) {
    update({ enabled, rules: nextRules ?? value.rules });
  }

  function updateRule(ruleId: string, next: SidebarTaskColorRule) {
    updateSettings(value.rules.map((rule) => (rule.id === ruleId ? next : rule)));
  }

  function addRule() {
    if (value.rules.length >= MAX_AUTOMATIC_TASK_COLOR_RULES) return;
    updateSettings([...value.rules, createAutomaticColorRule(value.rules.length + 1)]);
  }

  function removeRule(ruleId: string) {
    updateSettings(value.rules.filter((rule) => rule.id !== ruleId));
  }

  function reorderRules(activeId: string, overId: string) {
    const oldIndex = value.rules.findIndex((rule) => rule.id === activeId);
    const newIndex = value.rules.findIndex((rule) => rule.id === overId);
    if (oldIndex < 0 || newIndex < 0 || oldIndex === newIndex) return;
    updateSettings(arrayMove(value.rules, oldIndex, newIndex));
  }

  function setRepositorySearch(query: string) {
    setRepositoryQuery(query);
    repositoryCatalog.setQuery(query);
  }

  function openRepository(ruleId: string) {
    setActiveRepositoryRuleId(ruleId);
    setRepositorySearch("");
  }

  function closeRepository() {
    setActiveRepositoryRuleId(null);
    setRepositorySearch("");
  }

  return {
    workspaceId,
    value,
    updateRule,
    updateSettings,
    addRule,
    removeRule,
    reorderRules,
    saving,
    error,
    scalarOptions,
    expanded,
    setExpanded,
    activeRepositoryRule: value.rules.find((rule) => rule.id === activeRepositoryRuleId),
    repositoryQuery,
    setRepositorySearch,
    openRepository,
    closeRepository,
    repositoryCatalog,
  };
}

export type AutomaticColorSettingsController = ReturnType<
  typeof useAutomaticColorSettingsController
>;

function findUnavailableTargets(
  rules: readonly SidebarTaskColorRule[],
): { target: RepositoryRuleTarget; label: string }[] {
  return rules.flatMap((rule) => {
    if (
      rule.condition.dimension !== "repository" ||
      !isRepositoryRuleTarget(rule.condition.value)
    ) {
      return [];
    }
    return [{ target: rule.condition.value, label: rule.condition.label }];
  });
}
