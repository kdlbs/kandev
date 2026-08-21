import type { TFunction } from "i18next";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export function gitlabReviewWatchPlaceholders(t: TFunction): ScriptPlaceholder[] {
  return [
    {
      key: "mr.url",
      description: t("gitlab:placeholderActionUrl"),
      example: "https://gitlab.com/group/project/-/merge_requests/42",
      executor_types: [],
    },
    {
      key: "mr.title",
      description: t("gitlab:placeholderActionTitle"),
      example: "Fix login page crash",
      executor_types: [],
    },
    {
      key: "mr.project",
      description: t("gitlab:placeholderProjectPath"),
      example: "group/project",
      executor_types: [],
    },
    {
      key: "mr.iid",
      description: t("gitlab:placeholderMergeRequestIid"),
      example: "42",
      executor_types: [],
    },
    {
      key: "mr.description",
      description: t("gitlab:placeholderDescription"),
      example: "When clicking login...",
      executor_types: [],
    },
  ];
}
