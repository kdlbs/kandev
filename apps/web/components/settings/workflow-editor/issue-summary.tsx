"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import type { WorkflowEditorIssue } from "@/lib/workflows/workflow-editor-view-model";

export function IssueSummary({
  issues,
  onIssue,
}: {
  issues: WorkflowEditorIssue[];
  onIssue: (issue: WorkflowEditorIssue) => void;
}) {
  const { t } = useTranslation();
  if (issues.length === 0) return null;
  return (
    <section
      className="rounded-lg border border-destructive/40 bg-destructive/5 p-3"
      data-testid="workflow-editor-issues"
    >
      <div className="mb-2 flex items-center gap-2 text-sm font-medium text-destructive">
        <IconAlertTriangle className="h-4 w-4" />
        {t("workflows:configurationIssues")}
      </div>
      <div className="grid gap-1">
        {issues.map((issue) => (
          <button
            key={issue.id}
            type="button"
            className="min-h-11 cursor-pointer rounded px-2 text-left text-sm hover:bg-destructive/10"
            onClick={() => onIssue(issue)}
          >
            {t(issue.messageKey)}
          </button>
        ))}
      </div>
    </section>
  );
}
