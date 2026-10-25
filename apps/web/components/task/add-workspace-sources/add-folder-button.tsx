import { IconFolderPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { getWorkspaceSourceCapabilities } from "@/components/workspace-source-picker/executor-capabilities";
import { type WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";

export function AddFolderButton({
  capabilities,
  onAdd,
}: {
  isMobile: boolean;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  onAdd: (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) => void;
}) {
  const { t } = useTranslation();
  if (!capabilities.canAddFolders && capabilities.executorCapabilitiesKnown) return null;
  return (
    <Button
      type="button"
      variant="outline"
      disabled={!capabilities.canAddFolders}
      className="cursor-pointer"
      onClick={() => onAdd("folder")}
    >
      <IconFolderPlus className="h-4 w-4" />
      <span className="flex flex-col items-start">
        <span>{t("task:addFolder")}</span>
        {!capabilities.executorCapabilitiesKnown && (
          <span className="text-xs text-muted-foreground normal-case">
            {t("task:addFolderExecutorUnknown")}
          </span>
        )}
      </span>
    </Button>
  );
}
