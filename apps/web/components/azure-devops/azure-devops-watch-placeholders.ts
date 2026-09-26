import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export function azureWorkItemWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "work_item.url",
      description: t("azuredevops:placeholderUrlDescription"),
      example: "https://dev.azure.com/acme/Platform/_workitems/edit/42",
      executor_types: [],
    },
    {
      key: "work_item.title",
      description: t("azuredevops:placeholderTitleDescription"),
      example: "Fix login page crash",
      executor_types: [],
    },
    {
      key: "work_item.project",
      description: t("azuredevops:placeholderProjectDescription"),
      example: "Platform",
      executor_types: [],
    },
    {
      key: "work_item.id",
      description: t("azuredevops:placeholderIdDescription"),
      example: "42",
      executor_types: [],
    },
    {
      key: "work_item.description",
      description: t("azuredevops:placeholderDescriptionBody"),
      example: "When clicking login...",
      executor_types: [],
    },
    {
      key: "work_item.state",
      description: t("azuredevops:placeholderStateDescription"),
      example: "Active",
      executor_types: [],
    },
    {
      key: "work_item.type",
      description: t("azuredevops:placeholderTypeDescription"),
      example: "Bug",
      executor_types: [],
    },
  ];
}

export function azurePullRequestWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "pull_request.url",
      description: t("azuredevops:placeholderUrlDescription"),
      example: "https://dev.azure.com/acme/Platform/_git/app/pullrequest/42",
      executor_types: [],
    },
    {
      key: "pull_request.title",
      description: t("azuredevops:placeholderTitleDescription"),
      example: "Fix login page crash",
      executor_types: [],
    },
    {
      key: "pull_request.project",
      description: t("azuredevops:placeholderProjectDescription"),
      example: "Platform",
      executor_types: [],
    },
    {
      key: "pull_request.id",
      description: t("azuredevops:placeholderIdDescription"),
      example: "42",
      executor_types: [],
    },
    {
      key: "pull_request.description",
      description: t("azuredevops:placeholderDescriptionBody"),
      example: "When clicking login...",
      executor_types: [],
    },
    {
      key: "pull_request.state",
      description: t("azuredevops:placeholderStateDescription"),
      example: "Active",
      executor_types: [],
    },
    {
      key: "pull_request.source_branch",
      description: t("azuredevops:placeholderSourceBranchDescription"),
      example: "feature/login",
      executor_types: [],
    },
    {
      key: "pull_request.target_branch",
      description: t("azuredevops:placeholderTargetBranchDescription"),
      example: "main",
      executor_types: [],
    },
  ];
}
