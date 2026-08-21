import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export function gitlabIssueWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "issue.url",
      description: t("gitlab:placeholderActionUrl"),
      example: "https://gitlab.com/group/project/-/issues/42",
      executor_types: [],
    },
    {
      key: "issue.title",
      description: t("gitlab:placeholderActionTitle"),
      example: "Fix login page crash",
      executor_types: [],
    },
    {
      key: "issue.project",
      description: t("gitlab:placeholderProjectPath"),
      example: "group/project",
      executor_types: [],
    },
    {
      key: "issue.iid",
      description: t("gitlab:placeholderIssueIid"),
      example: "42",
      executor_types: [],
    },
    {
      key: "issue.description",
      description: t("gitlab:placeholderDescription"),
      example: "When clicking login...",
      executor_types: [],
    },
  ];
}
