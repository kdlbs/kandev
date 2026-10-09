"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsGroup } from "@/components/settings/settings-group";
import { useAppStore } from "@/components/state-provider";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { usePluginCapabilityApproval } from "./use-plugin-capability-approval";
import { PluginCapabilityApprovalEditor } from "./plugin-capability-approval-editor";

const MANAGE_SCOPE = "workspace.manage";

export function PluginCapabilityApproval({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const manageableWorkspaces = useMemo(
    () => workspaces.filter((workspace) => workspace.scopes?.includes(MANAGE_SCOPE)),
    [workspaces],
  );
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState(activeWorkspaceId ?? "");

  useEffect(() => {
    if (manageableWorkspaces.some((workspace) => workspace.id === selectedWorkspaceId)) return;
    const nextId =
      (manageableWorkspaces.some((workspace) => workspace.id === activeWorkspaceId)
        ? activeWorkspaceId
        : manageableWorkspaces[0]?.id) ?? "";
    setSelectedWorkspaceId(nextId);
  }, [activeWorkspaceId, manageableWorkspaces, selectedWorkspaceId]);

  if (manageableWorkspaces.length === 0) return null;
  const selectedWorkspace = manageableWorkspaces.find(
    (workspace) => workspace.id === selectedWorkspaceId,
  );

  return (
    <SettingsGroup
      title={t("plugins:capabilityApproval")}
      description={t("plugins:capabilityApprovalDescription")}
      data-testid="plugin-capability-approval"
      contentClassName="space-y-4"
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="plugin-approval-workspace">{t("plugins:approvalWorkspace")}</Label>
          <Select value={selectedWorkspaceId} onValueChange={setSelectedWorkspaceId}>
            <SelectTrigger
              id="plugin-approval-workspace"
              data-testid="plugin-approval-workspace"
              className={controlSizingClassName("standard", "w-full sm:max-w-md")}
            >
              <SelectValue placeholder={t("plugins:selectApprovalWorkspace")} />
            </SelectTrigger>
            <SelectContent>
              {manageableWorkspaces.map((workspace) => (
                <SelectItem key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {selectedWorkspace && (
          <ApprovalWorkspace
            key={selectedWorkspace.id}
            pluginId={pluginId}
            workspaceId={selectedWorkspace.id}
            workspaceName={selectedWorkspace.name}
          />
        )}
      </div>
    </SettingsGroup>
  );
}

function ApprovalWorkspace({
  pluginId,
  workspaceId,
  workspaceName,
}: {
  pluginId: string;
  workspaceId: string;
  workspaceName: string;
}) {
  const { t } = useTranslation();
  const approval = usePluginCapabilityApproval(pluginId, workspaceId);

  if (approval.loading) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {t("plugins:loadingCapabilityApproval")}
      </p>
    );
  }
  if (!approval.context) {
    return (
      <div className="space-y-2" data-testid="plugin-approval-load-error">
        <p className="text-sm text-destructive" role="alert">
          {t(approval.errorKey ?? "plugins:failedToLoadCapabilityApproval")}
        </p>
        <Button
          variant="outline"
          className={controlSizingClassName(
            "standard",
            "cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
          )}
          onClick={() => void approval.reload()}
        >
          {t("plugins:retryCapabilityApproval")}
        </Button>
      </div>
    );
  }
  return <PluginCapabilityApprovalEditor workspaceName={workspaceName} approval={approval} />;
}
