"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconBrandSentry, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";
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

// EditMode is the mutually-exclusive form state: at most one add-or-edit form
// is open at a time.
type EditMode = { kind: "none" } | { kind: "add" } | { kind: "edit"; id: string };

// useInstanceList loads and polls a workspace's Sentry instances. Fetches are
// request-versioned so a slow load for a previous workspace (or an overlapping
// poll) can't clobber newer results.
function useInstanceList(workspaceId: string) {
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
            description: `Failed to load Sentry instances: ${String(err)}`,
            variant: "error",
          });
        }
      } finally {
        if (current === requestId.current) setLoading(false);
      }
    },
    [workspaceId, toast],
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
};

function InstanceList({
  instances,
  mode,
  workspaceId,
  onEdit,
  onDelete,
  onSaved,
  onCancel,
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
  return (
    <div className="flex items-center gap-2 rounded-full border bg-muted/30 px-3 py-1">
      <Switch
        id="sentry-enabled"
        checked={enabled}
        onCheckedChange={setEnabled}
        className="cursor-pointer"
      />
      <Label htmlFor="sentry-enabled" className="text-xs cursor-pointer">
        {enabled ? "Enabled" : "Disabled"}
      </Label>
    </div>
  );
}

export function SentryConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { instances, loading, reload } = useInstanceList(workspaceId);
  const { toast } = useToast();
  const [mode, setMode] = useState<EditMode>({ kind: "none" });

  const closeForm = useCallback(() => setMode({ kind: "none" }), []);
  const handleSaved = useCallback(async () => {
    setMode({ kind: "none" });
    await reload();
  }, [reload]);

  const handleDelete = useCallback(
    async (instance: SentryConfig) => {
      if (!confirm(`Remove Sentry instance "${instance.name}"?`)) return;
      try {
        await deleteSentryInstance(workspaceId, instance.id);
        toast({ description: "Sentry instance removed", variant: "success" });
        await reload();
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
    [workspaceId, toast, reload],
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
      <Card>
        <CardContent className="space-y-3 pt-6">
          <h3 className="text-sm font-semibold" data-testid="sentry-instances-heading">
            Instances
          </h3>
          {loading && instances.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">Loading…</p>
          ) : (
            <InstanceList
              instances={instances}
              mode={mode}
              workspaceId={workspaceId}
              onEdit={(id) => setMode({ kind: "edit", id })}
              onDelete={handleDelete}
              onSaved={handleSaved}
              onCancel={closeForm}
            />
          )}
          {mode.kind === "add" && (
            <SentryInstanceForm
              workspaceId={workspaceId}
              instance={null}
              idPrefix="sentry-add"
              onSaved={handleSaved}
              onCancel={closeForm}
            />
          )}
          {canAddInstance && (
            <Button
              type="button"
              variant="outline"
              onClick={() => setMode({ kind: "add" })}
              className="cursor-pointer gap-1"
              data-testid="sentry-add-instance-button"
            >
              <IconPlus className="h-4 w-4" />
              Add instance
            </Button>
          )}
        </CardContent>
      </Card>
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
