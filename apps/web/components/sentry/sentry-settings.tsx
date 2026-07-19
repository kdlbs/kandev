"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { IconBrandSentry, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { deleteSentryInstance, sentryInUseWatchCount } from "@/lib/api/domains/sentry-api";
import { qk } from "@/lib/query/keys";
import { sentryInstancesQueryOptions } from "@/lib/query/query-options/sentry";
import type { SentryConfig } from "@/lib/types/sentry";
import { SentryInstanceCard } from "./sentry-instance-card";
import { SentryInstanceForm } from "./sentry-instance-form";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";

// EditMode is the mutually-exclusive form state: at most one add-or-edit form
// is open at a time.
type EditMode = { kind: "none" } | { kind: "add" } | { kind: "edit"; id: string };

function upsertInstance(list: SentryConfig[] | undefined, saved: SentryConfig): SentryConfig[] {
  const current = list ?? [];
  const index = current.findIndex((instance) => instance.id === saved.id);
  if (index === -1) return [...current, saved];
  const next = [...current];
  next[index] = saved;
  return next;
}

// useInstanceList loads and polls a workspace's Sentry instances through Query
// so every Sentry surface shares the same server-state cache.
function useInstanceList(workspaceId: string) {
  const { toast } = useToast();
  const reportedErrorRef = useRef(false);
  const query = useQuery(sentryInstancesQueryOptions(workspaceId));

  useEffect(() => {
    if (!query.isError) {
      reportedErrorRef.current = false;
      return;
    }
    if (reportedErrorRef.current) return;
    reportedErrorRef.current = true;
    toast({
      description: `Failed to load Sentry instances: ${String(query.error)}`,
      variant: "error",
    });
  }, [query.error, query.isError, toast]);

  return {
    instances: query.data ?? [],
    loading: query.isPending,
  };
}

type InstanceListProps = {
  instances: SentryConfig[];
  mode: EditMode;
  workspaceId: string;
  onEdit: (id: string) => void;
  onDelete: (instance: SentryConfig) => void;
  onSaved: (saved: SentryConfig) => void;
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
  if (instances.length === 0 && mode.kind !== "add") {
    return (
      <p className="text-sm text-muted-foreground py-2" data-testid="sentry-no-instances">
        No Sentry instances yet. Add one to connect this workspace.
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

function EnabledPill() {
  const { enabled, setEnabled } = useSentryEnabled();
  return <DraftedIntegrationEnabledControl id="sentry" enabled={enabled} persist={setEnabled} />;
}

function AddInstanceButton({ onAdd }: { onAdd: () => void }) {
  return (
    <Button
      type="button"
      variant="outline"
      onClick={onAdd}
      className="cursor-pointer gap-1"
      data-testid="sentry-add-instance-button"
    >
      <IconPlus className="h-4 w-4" />
      Add instance
    </Button>
  );
}

function useSentryInstanceActions(
  workspaceId: string,
  setMode: Dispatch<SetStateAction<EditMode>>,
  setFormDirty: Dispatch<SetStateAction<boolean>>,
) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const closeForm = useCallback(() => {
    setMode({ kind: "none" });
    setFormDirty(false);
  }, []);
  const handleSaved = useCallback(
    (saved: SentryConfig) => {
      queryClient.setQueryData<SentryConfig[]>(
        qk.integrations.sentry.instances(workspaceId),
        (current) => upsertInstance(current, saved),
      );
      void queryClient.invalidateQueries({
        queryKey: qk.integrations.sentry.instances(workspaceId),
      });
      setMode({ kind: "none" });
      setFormDirty(false);
    },
    [queryClient, workspaceId],
  );

  const handleDelete = useCallback(
    async (instance: SentryConfig) => {
      if (!confirm(`Remove Sentry instance "${instance.name}"?`)) return;
      try {
        await deleteSentryInstance(workspaceId, instance.id);
        queryClient.setQueryData<SentryConfig[]>(
          qk.integrations.sentry.instances(workspaceId),
          (current) => (current ?? []).filter((item) => item.id !== instance.id),
        );
        void queryClient.invalidateQueries({
          queryKey: qk.integrations.sentry.instances(workspaceId),
        });
        toast({ description: "Sentry instance removed", variant: "success" });
      } catch (err) {
        const watchCount = sentryInUseWatchCount(err);
        if (watchCount !== null) {
          const plural = watchCount === 1 ? "watch" : "watches";
          toast({
            description: `Can't delete "${instance.name}": ${watchCount} ${plural} still bound to it. Reassign or remove those watchers first.`,
            variant: "error",
          });
          return;
        }
        toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
      }
    },
    [queryClient, workspaceId, toast],
  );

  return { closeForm, handleSaved, handleDelete };
}

export function SentryConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { instances, loading } = useInstanceList(workspaceId);
  const [mode, setMode] = useState<EditMode>({ kind: "none" });
  const [formDirty, setFormDirty] = useState(false);
  const { closeForm, handleSaved, handleDelete } = useSentryInstanceActions(
    workspaceId,
    setMode,
    setFormDirty,
  );
  const canAddInstance =
    mode.kind === "none" ||
    (mode.kind === "edit" && !instances.some((instance) => instance.id === mode.id));

  return (
    <SettingsSection
      icon={<IconBrandSentry className="h-5 w-5" />}
      title="Sentry integration"
      description="Connect this workspace to Sentry. Add a named instance for each Sentry org or self-hosted host; credentials are stored encrypted server-side."
      action={<EnabledPill />}
    >
      <SettingsCard isDirty={formDirty}>
        <CardContent className="space-y-3 pt-6">
          <h3 className="text-sm font-semibold" data-testid="sentry-instances-heading">
            Instances
          </h3>
          {loading && instances.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">Loading...</p>
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
            <AddInstanceButton
              onAdd={() => {
                setFormDirty(false);
                setMode({ kind: "add" });
              }}
            />
          )}
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}

type SentryIntegrationPageProps = {
  workspaceId?: string;
};

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
