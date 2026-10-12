"use client";

import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconPlus } from "@tabler/icons-react";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useCoordinators } from "@/hooks/domains/settings/use-coordinators";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import {
  resolveAgentProfileLabel,
  resolveExecutorProfileLabel,
} from "@/lib/coordinators/profile-lookup";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { CoordinatorCard } from "./coordinator-card";
import type { WorkspaceState } from "@/lib/state/slices";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";

type Workspace = WorkspaceState["items"][number];

type CoordinatorsListPageProps = {
  workspaceId: string;
};

type CoordinatorsListBodyProps = {
  loading: boolean;
  loaded: boolean;
  loadError: boolean;
  refresh: () => void;
  items: Coordinator[];
  agentProfiles: AgentProfileOption[];
  executors: Executor[];
  missingLabel: string;
  workspaceId: string;
};

// Avoids a nested ternary over the loading/error/empty/list states.
function CoordinatorsListBody({
  loading,
  loaded,
  loadError,
  refresh,
  items,
  agentProfiles,
  executors,
  missingLabel,
  workspaceId,
}: CoordinatorsListBodyProps) {
  const { t } = useTranslation();

  if (loading && !loaded) {
    return (
      <div className="py-12 text-center text-muted-foreground" data-testid="coordinators-loading">
        {t("coordinator:loadingCoordinators")}
      </div>
    );
  }

  if (loadError) {
    return (
      <div
        className="space-y-4 py-12 text-center text-muted-foreground"
        data-testid="coordinators-load-error"
      >
        <p>{t("coordinator:loadError")}</p>
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          data-testid="coordinators-retry-button"
          onClick={refresh}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <p className="py-12 text-center text-muted-foreground" data-testid="coordinators-empty-state">
        {t("coordinator:emptyState")}
      </p>
    );
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {items.map((coordinator) => (
        <CoordinatorCard
          key={coordinator.id}
          coordinator={coordinator}
          agentProfileLabel={resolveAgentProfileLabel(
            coordinator.agent_profile_id,
            agentProfiles,
            missingLabel,
          )}
          executorProfileLabel={resolveExecutorProfileLabel(
            coordinator.executor_profile_id,
            executors,
            missingLabel,
          )}
          openHref={`/workspaces/${workspaceId}/coordinator/${coordinator.id}`}
          configureHref={`/settings/workspaces/${workspaceId}/coordinators/${coordinator.id}`}
        />
      ))}
    </div>
  );
}

export function CoordinatorsListPage({ workspaceId }: CoordinatorsListPageProps) {
  const phase2 = useFeature("coordinatorPhase2");
  const { t } = useTranslation();
  const router = useRouter();
  useSettingsData(true);
  const { items, loaded, loading, loadError, refresh } = useCoordinators(workspaceId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);
  const missingLabel = t("coordinator:profileRemoved");

  return (
    <div className="space-y-6" data-testid="coordinators-list-page">
      <WorkspaceSectionHeader
        tab="coordinators"
        description={t("coordinator:listDescription")}
        action={
          canManage ? (
            <Button
              type="button"
              data-testid="add-coordinator-button"
              className={controlSizingClassName("standard", "cursor-pointer")}
              onClick={() => router.push(`/settings/workspaces/${workspaceId}/coordinators/new`)}
            >
              <IconPlus className="h-4 w-4 mr-2" />
              {t("coordinator:addCoordinator")}
            </Button>
          ) : undefined
        }
      />
      <Separator />
      <CoordinatorsListBody
        loading={loading}
        loaded={loaded}
        loadError={loadError}
        refresh={refresh}
        items={items}
        agentProfiles={agentProfiles}
        executors={executors}
        missingLabel={missingLabel}
        workspaceId={workspaceId}
      />
      {!phase2 && (
        <p
          className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground"
          data-testid="coordinators-later-phase-note"
        >
          {t("coordinator:laterPhaseNote")}
        </p>
      )}
    </div>
  );
}
