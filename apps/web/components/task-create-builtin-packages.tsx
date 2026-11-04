import { useEffect, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  BuiltinPackageDetail,
  BuiltinPackageList,
  type BuiltinMCPEditor,
} from "@/components/mcp/builtin-package-detail";
import { useBuiltinMCPManagement } from "@/hooks/domains/workspace/use-builtin-mcp-management";
import type {
  BuiltinMCPCreateDraft,
  BuiltinMCPPackage,
  BuiltinMCPPreference,
} from "@/lib/types/builtin-mcp";

export function TaskCreateBuiltinPackages({
  workspaceId,
  draft,
  onChange,
}: {
  workspaceId: string;
  draft?: BuiltinMCPCreateDraft;
  onChange: (draft: BuiltinMCPCreateDraft | undefined) => void;
}) {
  const { t } = useTranslation();
  const remote = useBuiltinMCPManagement("workspaces", workspaceId, workspaceId);
  const [selected, select] = useState<BuiltinMCPPackage | null>(null);
  const returnFocus = useRef<BuiltinMCPPackage | null>(null);
  if (selected) returnFocus.current = selected;
  useEffect(() => {
    onChange(undefined);
    select(null);
  }, [workspaceId, onChange]);
  const choices = draft?.workspaceId === workspaceId ? draft.preferences.packages : {};
  const view = remote.view
    ? {
        ...remote.view,
        surface: "kanban-task" as const,
        can_edit: remote.view.can_choose_for_task === true,
      }
    : null;
  const editor: BuiltinMCPEditor = {
    ...remote,
    view,
    draft: choices,
    setChoice: (id, choice) => {
      if (
        !view?.can_edit ||
        remote.pending ||
        id === "core" ||
        !["inherit", "on", "off"].includes(choice ?? "")
      )
        return;
      onChange({
        workspaceId,
        preferences: {
          version: 1,
          expected_workspace_policy_revision: view.policy.revision,
          packages: { ...choices, [id]: choice as BuiltinMCPPreference },
        },
      });
    },
  };
  const item = view?.catalog.find((entry) => entry.id === selected);
  return (
    <details className="rounded-md border p-3" data-testid="task-create-builtin-packages">
      <summary className="min-h-7 cursor-pointer text-sm font-medium max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        {t("settings:builtinMcp.title")}
      </summary>
      <div className="space-y-3 pt-3">
        {item ? (
          <BuiltinPackageDetail
            editor={editor}
            item={item}
            scope="tasks"
            onBack={() => select(null)}
          />
        ) : (
          <BuiltinPackageList returnFocus={returnFocus.current} editor={editor} onSelect={select} />
        )}
        <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.creationConsent")}</p>
      </div>
    </details>
  );
}
