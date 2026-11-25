"use client";

import { useTranslation } from "react-i18next";
import { SettingsGroup } from "@/components/settings/settings-group";
import { WorkspacePlacementCard } from "@/components/settings/workspaces/workspace-placement-card";
import { WorkspaceTeamAccessCard } from "@/components/settings/workspaces/workspace-team-access-card";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import type { WorkspaceState } from "@/lib/state/slices";

type Workspace = WorkspaceState["items"][number];

export function WorkspaceAccessSettings({ workspace }: { workspace: Workspace }) {
  const signedIn = useAppStore(
    (state) => state.auth.mode === "enabled" && state.auth.authenticated,
  );
  const { t } = useTranslation();
  if (!signedIn) return null;

  const title = t("workspaces:accessSettings.title");
  return (
    <>
      <SettingsGroup
        title={title}
        aria-label={title}
        description={t("workspaces:accessSettings.description")}
        frame="none"
        contentClassName="space-y-4"
      >
        <WorkspacePlacementCard
          workspaceId={workspace.id}
          unitId={workspace.unit_id}
          scopes={workspace.scopes}
        />
        <WorkspaceTeamAccessCard
          workspaceId={workspace.id}
          ownerId={workspace.owner_id ?? ""}
          scopes={workspace.scopes}
        />
      </SettingsGroup>
      <Separator />
    </>
  );
}
