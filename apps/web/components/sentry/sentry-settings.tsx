"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconBrandSentry, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { SentryEnabledControl } from "@/components/sentry/sentry-enabled-control";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  deleteSentryInstance,
  listSentryInstances,
  sentryInUseWatchCount,
} from "@/lib/api/domains/sentry-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { SentryInstanceCard } from "./sentry-instance-card";
import { SentryInstanceForm } from "./sentry-instance-form";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";
import { useTranslation } from "react-i18next";
import { INTEGRATION_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/integrations";

// EditMode is the mutually-exclusive form state: at most one add-or-edit form
// is open at a time.
type EditMode = { kind: "none" } | { kind: "add" } | { kind: "edit"; id: string };

// useInstanceList loads and polls a workspace's Sentry instances. Fetches are
// request-versioned so a slow load for a previous workspace (or an overlapping
// poll) can't clobber newer results.
function useInstanceList(workspaceId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const requestId = useRef(0);

  const reload = useCallback(
    async (reportError = true) => {
      const current = ++requestId.current;
      try {
        const list = await listSentryInstances(workspaceId);
        if (current !== requestId.current) return;
        setInstances(list);
      } catch (err) {
        if (current !== requestId.current) return;
        if (reportError) {
          toast({
            description: t("sentry:failedToLoadInstances", { error: String(err) }),
            variant: "error",
          });
        }
      } finally {
        if (current === requestId.current) setLoading(false);
      }
    },
    [workspaceId, toast, t],
  );

  useEffect(() => {
    setLoading(true);
    setInstances([]);
    void reload();
    // Poll so per-instance health banners stay fresh without a manual refresh.
    const id = setInterval(() => void reload(false), INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [reload]);

  return { instances, loading, reload };
}

type InstanceListProps = {
  instances: SentryConfig[];
  mode: EditMode;
  workspaceId: string;
  onEdit: (id: string) => void;
  onDelete: (instance: SentryConfig) => void;
  onSaved: () => void;
  onCancel: () => void;
  onDirtyChange: (isDirty: boolean) => void;
};

function InstanceList({
  instances,
  mode,
  workspaceId,
  onEdit,
  onDelete,
  onSaved,
  onCancel,
  onDirtyChange,
}: InstanceListProps) {
  const { t } = useTranslation();
  if (instances.length === 0 && mode.kind !== "add") {
    return (
      <p className="text-sm text-muted-foreground py-2" data-testid="sentry-no-instances">
        {t("sentry:noInstancesYet")}
      </p>
    );
  }
  return (
    <div className="space-y-3">
      {instances.map((instance) =>
        mode.kind === "edit" && mode.id === instance.id ? (
          <SentryInstanceForm
            key={instance.id}
            workspaceId={workspaceId}
            instance={instance}
            idPrefix="sentry-edit"
            onSaved={onSaved}
            onCancel={onCancel}
            onDirtyChange={onDirtyChange}
          />
        ) : (
          <SentryInstanceCard
            key={instance.id}
            instance={instance}
            onEdit={() => onEdit(instance.id)}
            onDelete={() => onDelete(instance)}
          />
        ),
      )}
    </div>
  );
}

// useDeleteInstance confirms, deletes, and reports the outcome. Split out of
// SentryConnectionSection so that component stays under the file's
// max-lines-per-function limit.
function useDeleteInstance(workspaceId: string, reload: () => Promise<void>) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(
    async (instance: SentryConfig) => {
      if (!confirm(t("sentry:removeInstanceConfirm", { name: instance.name }))) return;
      try {
        await deleteSentryInstance(workspaceId, instance.id);
        toast({ description: t("sentry:instanceRemoved"), variant: "success" });
        await reload();
      } catch (err) {
        const watchCount = sentryInUseWatchCount(err);
        if (watchCount !== null) {
          // `count` drives i18next's plural selection; writing the "watch" /
          // "watches" ending at the call site would make the message
          // untranslatable for every language with a different plural rule.
          toast({
            description: t("sentry:cantDeleteInstanceInUse", {
              name: instance.name,
              count: watchCount,
            }),
            variant: "error",
          });
          return;
        }
        toast({ description: t("sentry:deleteFailed", { error: String(err) }), variant: "error" });
      }
    },
    [workspaceId, toast, reload, t],
  );
}

/** Sentry connection card: title/enable toggle plus the list of configured Sentry instances. */
export function SentryConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { instances, loading, reload } = useInstanceList(workspaceId);
  const [mode, setMode] = useState<EditMode>({ kind: "none" });
  const [formDirty, setFormDirty] = useState(false);

  const closeForm = useCallback(() => {
    setMode({ kind: "none" });
    setFormDirty(false);
  }, []);
  const handleSaved = useCallback(async () => {
    setMode({ kind: "none" });
    setFormDirty(false);
    await reload();
  }, [reload]);

  const handleDelete = useDeleteInstance(workspaceId, reload);

  const canAddInstance =
    mode.kind === "none" ||
    (mode.kind === "edit" && !instances.some((instance) => instance.id === mode.id));

  return (
    <SettingsSection
      discoveryTargetId={INTEGRATION_SETTINGS_TARGETS.sentry}
      icon={<IconBrandSentry className="h-5 w-5" />}
      title={t("sentry:sentryIntegration")}
      description={t("sentry:sentryIntegrationDescription")}
      action={<SentryEnabledControl />}
    >
      <SettingsCard isDirty={formDirty}>
        <CardContent className="space-y-3 pt-6">
          <h3 className="text-sm font-semibold" data-testid="sentry-instances-heading">
            {t("sentry:instances")}
          </h3>
          {loading && instances.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              {t("sentry:loadingEllipsis")}
            </p>
          ) : (
            <InstanceList
              instances={instances}
              mode={mode}
              workspaceId={workspaceId}
              onEdit={(id) => {
                setFormDirty(false);
                setMode({ kind: "edit", id });
              }}
              onDelete={handleDelete}
              onSaved={handleSaved}
              onCancel={closeForm}
              onDirtyChange={setFormDirty}
            />
          )}
          {mode.kind === "add" && (
            <SentryInstanceForm
              workspaceId={workspaceId}
              instance={null}
              idPrefix="sentry-add"
              onSaved={handleSaved}
              onCancel={closeForm}
              onDirtyChange={setFormDirty}
            />
          )}
          {canAddInstance && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setFormDirty(false);
                setMode({ kind: "add" });
              }}
              className="cursor-pointer gap-1"
              data-testid="sentry-add-instance-button"
            >
              <IconPlus className="h-4 w-4" />
              {t("sentry:addInstance")}
            </Button>
          )}
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}

type SentryIntegrationPageProps = {
  workspaceId?: string;
};

/** Sentry's own settings page: connection card plus issue-watchers section. */
export function SentryIntegrationPage({ workspaceId }: SentryIntegrationPageProps = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(resolvedWorkspaceId) => (
          <div key={resolvedWorkspaceId} className="space-y-8">
            <SentryConnectionSection workspaceId={resolvedWorkspaceId} />
            <SentryIssueWatchersSection workspaceId={resolvedWorkspaceId} />
          </div>
        )}
      </WorkspaceScopedSection>
    </div>
  );
}
