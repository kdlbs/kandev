"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  AgentUpdateJob,
  AgentUpdateMode,
  AgentUpdatePreview,
  AgentUpdateStatus,
  InstallJob,
} from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import {
  buildRuntimeUpdateManagerModel,
  runtimePolicyKey,
  type RuntimeUpdateGroup,
} from "@/lib/agents/runtime-update-manager";
import { useSettingsSaveCoordinator, type SettingsSaveCoordinator } from "./settings-save-provider";
import { RuntimePolicyDetail, useRuntimePolicyControllers } from "./agent-runtime-policies";
import { RuntimePolicyDirtyFooter } from "./agent-runtime-policy-footer";
import { AgentRuntimeUpdateDetail } from "./agent-runtime-update-detail";
import { RuntimeUpdateGuidanceSurface } from "./agent-runtime-update-guidance-surface";
import { RuntimeUpdateOverviewSurface } from "./agent-runtime-update-overview";
import { useRuntimeUpdateReview } from "@/hooks/domains/settings/use-runtime-update-batch-review";
import type { RuntimeUpdateManagerView } from "@/hooks/domains/settings/use-agent-runtime-update-manager";

type PreviewUpdate = (
  agentName: string,
  targetVersion?: string,
  useDefault?: boolean,
  targetFamily?: "v2",
) => Promise<AgentUpdatePreview>;

type StartUpdate = (
  agentName: string,
  targetVersion: string,
  useDefault?: boolean,
  targetFamily?: "v2" | AgentUpdateMode,
  expectedRuntimeRevision?: number,
) => Promise<AgentUpdateJob>;

export type AgentRuntimeUpdateManagerProps = {
  view: RuntimeUpdateManagerView;
  isMobile: boolean;
  statuses: AgentUpdateStatus[];
  statusLoading: boolean;
  refreshFailed: boolean;
  updateJobs: Record<string, AgentUpdateJob>;
  installJobs: Record<string, InstallJob>;
  canManage: boolean;
  hasRuntimeControl: (agentName: string) => boolean;
  resolveDisplayName: (agentName: string) => string;
  resolveRuntimeUpdate: (agentName: string) => RuntimeUpdate | undefined;
  previewUpdate: PreviewUpdate;
  startUpdate: StartUpdate;
  refresh: () => Promise<boolean>;
  onClose: () => void;
  onBack: () => void;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
};

function useRuntimeManagerActions({
  coordinator,
  startUpdate,
  refresh,
  refreshFailed,
}: {
  coordinator: SettingsSaveCoordinator;
  startUpdate: StartUpdate;
  refresh: () => Promise<boolean>;
  refreshFailed: boolean;
}) {
  const { t } = useTranslation();
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState(refreshFailed);

  useEffect(() => {
    if (refreshFailed) setRefreshError(true);
  }, [refreshFailed]);

  const admit = useCallback<StartUpdate>(
    async (...args) => {
      const result = await coordinator.runExclusive(() => startUpdate(...args));
      if (!result) throw new Error(t("agents:runtimePolicyBusy"));
      return result.value;
    },
    [coordinator, startUpdate, t],
  );

  const handleRefresh = useCallback(async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      setRefreshError(!(await refresh()));
    } finally {
      setRefreshing(false);
    }
  }, [refresh, refreshing]);

  return {
    admit,
    handleRefresh,
    refreshError,
    refreshing,
  };
}

function RuntimeManagerDetail({
  view,
  isMobile,
  model,
  controllers,
  canManage,
  updateJobs,
  installJobs,
  resolveDisplayName,
  resolveRuntimeUpdate,
  previewUpdate,
  admit,
  onBack,
  onClose,
  dirtyBlockedLabel,
}: {
  view: Extract<RuntimeUpdateManagerView, { kind: "runtime" }>;
  isMobile: boolean;
  model: ReturnType<typeof buildRuntimeUpdateManagerModel>;
  controllers: ReturnType<typeof useRuntimePolicyControllers>;
  canManage: boolean;
  updateJobs: Record<string, AgentUpdateJob>;
  installJobs: Record<string, InstallJob>;
  resolveDisplayName: (agentName: string) => string;
  resolveRuntimeUpdate: (agentName: string) => RuntimeUpdate | undefined;
  previewUpdate: PreviewUpdate;
  admit: StartUpdate;
  onBack: () => void;
  onClose: () => void;
  dirtyBlockedLabel: string;
}) {
  const agentName = view.agentName;
  const displayName = resolveDisplayName(agentName);
  const status = model.rows.find((row) => row.status.agent_name === agentName)?.status;
  const runtimeUpdate = resolveRuntimeUpdate(agentName);
  const dirty = controllers.some(
    (controller) => controller.agentName === agentName && controller.isDirty,
  );
  const dirtyFooter = (
    <RuntimePolicyDirtyFooter controllers={controllers} canManage={canManage} embedded />
  );
  if (!status || !runtimeUpdate?.supported) {
    return (
      <RuntimeUpdateGuidanceSurface
        agentName={agentName}
        displayName={displayName}
        status={status}
        isMobile={isMobile}
        onBack={onBack}
        onClose={onClose}
      />
    );
  }
  return (
    <AgentRuntimeUpdateDetail
      key={agentName}
      control={{
        agentName,
        displayName,
        runtimeUpdate,
        runtimeUpdateStatus: status,
        job: updateJobs[agentName],
        installJob: installJobs[agentName],
        canManage,
        onPreview: previewUpdate,
        onUpdate: admit,
      }}
      open
      onOpenChange={(open) => !open && onClose()}
      isMobile={isMobile}
      onBack={onBack}
      beforeBody={<RuntimePolicyDetail status={status} />}
      footerBefore={dirtyFooter}
      approveDisabledReason={dirty ? dirtyBlockedLabel : undefined}
    />
  );
}

function RuntimeManagerOverview({
  model,
  isMobile,
  canManage,
  statusLoading,
  refreshError,
  refreshing,
  onRefresh,
  onOpenRuntime,
  onClose,
  controllers,
  bulk,
  coordinator,
  initialScrollTop,
  onScrollTopChange,
  openGroups,
  onGroupOpenChange,
}: {
  model: ReturnType<typeof buildRuntimeUpdateManagerModel>;
  isMobile: boolean;
  canManage: boolean;
  statusLoading: boolean;
  refreshError: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
  onClose: () => void;
  controllers: ReturnType<typeof useRuntimePolicyControllers>;
  bulk: ReturnType<typeof useRuntimeUpdateReview>;
  coordinator: SettingsSaveCoordinator;
  initialScrollTop: number;
  onScrollTopChange: (scrollTop: number) => void;
  openGroups: ReadonlySet<string>;
  onGroupOpenChange: (group: RuntimeUpdateGroup, open: boolean) => void;
}) {
  return (
    <RuntimeUpdateOverviewSurface
      model={model}
      isMobile={isMobile}
      canManage={canManage}
      statusLoading={statusLoading}
      refreshFailed={refreshError}
      refreshing={refreshing}
      onRefresh={onRefresh}
      onOpenRuntime={onOpenRuntime}
      individualErrors={{}}
      onClose={onClose}
      onReviewAgain={bulk.resetAfterCompletion}
      initialScrollTop={initialScrollTop}
      onScrollTopChange={onScrollTopChange}
      openGroups={openGroups}
      onGroupOpenChange={onGroupOpenChange}
      bulk={{
        batch: bulk.batch,
        reviewRows: bulk.reviewRows,
        reviewKeys: bulk.reviewKeys,
        previewFailures: bulk.previewFailures,
        reviewLoading: bulk.reviewLoading,
        selection: bulk.selection,
        allSelected: bulk.allSelected,
        onToggleSelected: bulk.toggleSelected,
        onSelectAll: bulk.selectAll,
        onStart: (eligibleKeys) =>
          void coordinator.runExclusive(() => bulk.startSelected(eligibleKeys)),
        onRetryFailed: () => bulk.retryFailed(),
        onRefreshStatus: () => void bulk.refreshStatus(),
        onRetryPreviews: bulk.retryPreviews,
      }}
      footerBefore={
        <RuntimePolicyDirtyFooter controllers={controllers} canManage={canManage} embedded />
      }
    />
  );
}

function useRuntimeManagerModel({
  active,
  statuses,
  hasRuntimeControl,
  installJobs,
  updateJobs,
  previewUpdate,
}: {
  active: boolean;
  statuses: AgentUpdateStatus[];
  hasRuntimeControl: (agentName: string) => boolean;
  installJobs: Record<string, InstallJob>;
  updateJobs: Record<string, AgentUpdateJob>;
  previewUpdate: PreviewUpdate;
}) {
  const controllers = useRuntimePolicyControllers();
  const dirtyKeys = useMemo(
    () =>
      new Set(
        controllers
          .filter((controller) => controller.isDirty)
          .map((controller) => runtimePolicyKey(controller.status)),
      ),
    [controllers],
  );
  const model = useMemo(
    () =>
      buildRuntimeUpdateManagerModel(statuses, {
        hasRuntimeControl,
        installJobs,
        updateJobs,
        dirtyPolicyKeys: dirtyKeys,
      }),
    [statuses, hasRuntimeControl, installJobs, updateJobs, dirtyKeys],
  );
  const bulk = useRuntimeUpdateReview({
    active,
    candidates: model.bulkCandidates.map((row) => row.status),
    preview: previewUpdate,
    isEligible: (status) => !dirtyKeys.has(runtimePolicyKey(status)),
  });
  return { controllers, model, bulk };
}

/**
 * One manager surface for overview and runtime details. The page keeps this
 * component mounted across overview-to-detail navigation so selection and
 * policy drafts are not reconstructed; policy contributors live above it.
 */
export function AgentRuntimeUpdateManager(props: AgentRuntimeUpdateManagerProps) {
  const {
    view,
    isMobile,
    statuses,
    statusLoading,
    refreshFailed,
    updateJobs,
    installJobs,
    canManage,
    hasRuntimeControl,
    resolveDisplayName,
    resolveRuntimeUpdate,
    previewUpdate,
    startUpdate,
    refresh,
    onClose,
    onBack,
    onOpenRuntime,
  } = props;
  const { t } = useTranslation();
  const coordinator = useSettingsSaveCoordinator();
  const actions = useRuntimeManagerActions({
    coordinator,
    startUpdate,
    refresh,
    refreshFailed,
  });
  const overviewScrollTop = useRef(0);
  const [openGroups, setOpenGroups] = useState<ReadonlySet<string>>(new Set());
  const { controllers, model, bulk } = useRuntimeManagerModel({
    active: view.kind === "overview",
    statuses,
    hasRuntimeControl,
    installJobs,
    updateJobs,
    previewUpdate,
  });

  if (view.kind === "overview") {
    return (
      <RuntimeManagerOverview
        model={model}
        isMobile={isMobile}
        canManage={canManage}
        statusLoading={statusLoading}
        refreshError={actions.refreshError}
        refreshing={actions.refreshing}
        onRefresh={() => void actions.handleRefresh()}
        onOpenRuntime={onOpenRuntime}
        onClose={onClose}
        controllers={controllers}
        bulk={bulk}
        coordinator={coordinator}
        initialScrollTop={overviewScrollTop.current}
        onScrollTopChange={(scrollTop) => {
          overviewScrollTop.current = scrollTop;
        }}
        openGroups={openGroups}
        onGroupOpenChange={(group, open) =>
          setOpenGroups((previous) => {
            const next = new Set(previous);
            if (open) next.add(group);
            else next.delete(group);
            return next;
          })
        }
      />
    );
  }
  if (view.kind !== "runtime") return null;

  return (
    <RuntimeManagerDetail
      view={view}
      isMobile={isMobile}
      model={model}
      controllers={controllers}
      canManage={canManage}
      updateJobs={updateJobs}
      installJobs={installJobs}
      resolveDisplayName={resolveDisplayName}
      resolveRuntimeUpdate={resolveRuntimeUpdate}
      previewUpdate={previewUpdate}
      admit={actions.admit}
      onBack={onBack}
      onClose={onClose}
      dirtyBlockedLabel={t("agents:runtimePolicyDirtyBlocked")}
    />
  );
}
