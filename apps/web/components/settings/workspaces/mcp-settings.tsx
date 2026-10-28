"use client";
import { MCPAssignmentChooser } from "./mcp-assignment-chooser";
import type { MCPRegistrationReceipt } from "@/hooks/domains/workspace/use-mcp-registration-completion";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { IconPlus, IconServer, IconTrash } from "@tabler/icons-react";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useToast } from "@/components/toast-provider";
import { useMCPWorkspaceSettings } from "@/hooks/domains/workspace/use-mcp-workspace-settings";
import type {
  MCPDefinitionInput,
  MCPServerDefinition,
  MCPSelectionImpact,
} from "@/lib/types/http-mcp";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { mcpCatalogUsageHref, readMCPContextHint } from "@/lib/mcp/management-links";
import { MCPUsage } from "./mcp-usage";
import { SettingsSaveCancelledError } from "@/components/settings/settings-save-provider";
import { MCPRegistrationCompletion, MCPRegistrationUncertain } from "./mcp-registration-completion";
import type { MCPRegistrationResult } from "@/hooks/domains/workspace/use-mcp-registration-completion";
import { MCPDefinitionForm } from "./mcp-definition-form";
import { MCPMarketplace } from "./mcp-marketplace";

type MCPSettingsProps = { workspaceId: string };

const STANDARD_ACTION_CLASS = controlSizingClassName("standard", "cursor-pointer");
const METADATA_BADGE_CLASS =
  "h-auto min-h-6 max-w-full whitespace-normal border-border bg-muted px-2 py-0.5 text-xs font-medium leading-4 text-foreground dark:bg-muted";

function modeLabel(mode: string, t: (key: string) => string): string {
  if (mode === "remote") return t("settings:mcpModeRemote");
  if (mode === "managed_package") return t("settings:mcpModeManagedPackage");
  return t("settings:mcpModeExistingExecutable");
}

function transportLabel(transport: string, t: (key: string) => string): string {
  if (transport === "stdio") return t("settings:mcpTransportStdio");
  if (transport === "sse") return t("settings:mcpTransportSse");
  return t("settings:mcpTransportStreamable");
}

function selectionImpactTotal(impact?: MCPSelectionImpact): number {
  if (!impact) return 0;
  return impact.profile + impact.repository + impact.task + impact.task_session;
}

function SelectionImpact({ impact }: { impact?: MCPSelectionImpact }) {
  const { t } = useTranslation();
  const total = selectionImpactTotal(impact);
  return (
    <p className="text-xs text-muted-foreground" data-testid="mcp-selection-impact">
      {total > 0
        ? t("settings:mcpSelectionImpact", {
            profile: t("settings:mcpProfileUsage", { count: impact?.profile ?? 0 }),
            repository: t("settings:mcpRepositoryUsage", { count: impact?.repository ?? 0 }),
            task: t("settings:mcpTaskUsage", { count: impact?.task ?? 0 }),
            taskSession: t("settings:mcpSessionUsage", { count: impact?.task_session ?? 0 }),
          })
        : t("settings:mcpSelectionImpactNone")}
    </p>
  );
}

function DefinitionBadges({ server }: { server: MCPServerDefinition }) {
  const { t } = useTranslation();
  return (
    <div data-testid="mcp-definition-badges" className="flex flex-wrap gap-1.5">
      <Badge variant="outline" className={METADATA_BADGE_CLASS}>
        {modeLabel(server.execution_mode, t)}
      </Badge>
      <Badge variant="outline" className={METADATA_BADGE_CLASS}>
        {transportLabel(server.transport, t)}
      </Badge>
      <Badge variant="outline" className={METADATA_BADGE_CLASS}>
        {server.source === "registry"
          ? t("settings:mcpSourceRegistry")
          : t("settings:mcpSourceCustom")}
      </Badge>
    </div>
  );
}

function ServerCard({
  server,
  onEdit,
  onToggle,
  onDelete,
  onUsage,
  onUse,
}: {
  server: MCPServerDefinition;
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
  onUsage: () => void;
  onUse: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Card data-testid={`mcp-server-card-${server.id}`} className="min-w-0">
      <CardHeader className="space-y-2">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <CardTitle className="min-w-0 truncate text-base">{server.display_name}</CardTitle>
          <Badge variant={server.enabled ? "default" : "outline"}>
            {server.enabled ? t("settings:mcpEnabled") : t("settings:mcpDisabled")}
          </Badge>
        </div>
        <p className="truncate text-xs text-muted-foreground">{server.runtime_name}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {server.description || t("settings:mcpNoDescription")}
        </p>
        <DefinitionBadges server={server} />
        <SelectionImpact impact={server.selection_impact} />
        {!server.enabled && (
          <p className="text-xs text-muted-foreground">{t("settings:mcpAssignmentDisabled")}</p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={STANDARD_ACTION_CLASS}
            onClick={onUse}
            disabled={!server.enabled}
          >
            {t("settings:mcpUseIn")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={STANDARD_ACTION_CLASS}
            onClick={onUsage}
          >
            {t("settings:mcpUsedIn")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={STANDARD_ACTION_CLASS}
            onClick={onEdit}
          >
            {t("common:edit")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={STANDARD_ACTION_CLASS}
            onClick={onToggle}
          >
            {server.enabled ? t("settings:mcpDisable") : t("settings:mcpEnable")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className={controlSizingClassName("icon", "cursor-pointer text-destructive")}
            onClick={onDelete}
            aria-label={t("settings:deleteMcpServer")}
          >
            <IconTrash className="size-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function EmptyCatalog({ onAdd, addDisabled }: { onAdd: () => void; addDisabled: boolean }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <IconServer className="mx-auto mb-3 size-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">{t("settings:mcpCatalogEmpty")}</p>
        <Button
          type="button"
          className={controlSizingClassName("standard", "mt-4 cursor-pointer")}
          onClick={onAdd}
          disabled={addDisabled}
        >
          <IconPlus className="mr-2 size-4" />
          {t("settings:addMcpServer")}
        </Button>
      </CardContent>
    </Card>
  );
}

function ConfiguredMCPServers({
  servers,
  loading,
  isMobile,
  onAdd,
  addDisabled,
  onEdit,
  onToggle,
  onDelete,
  onUsage,
  onUse,
}: {
  servers: MCPServerDefinition[];
  loading: boolean;
  isMobile: boolean;
  onAdd: () => void;
  addDisabled: boolean;
  onEdit: (server: MCPServerDefinition) => void;
  onToggle: (server: MCPServerDefinition) => void;
  onDelete: (server: MCPServerDefinition) => void;
  onUsage: (server: MCPServerDefinition) => void;
  onUse: (server: MCPServerDefinition) => void;
}) {
  if (servers.length === 0 && !loading)
    return <EmptyCatalog onAdd={onAdd} addDisabled={addDisabled} />;
  const gridClass = isMobile ? "grid grid-cols-1 gap-3" : "grid grid-cols-1 gap-4 xl:grid-cols-2";
  return (
    <div className={gridClass}>
      {servers.map((server) => (
        <ServerCard
          key={server.id}
          server={server}
          onEdit={() => onEdit(server)}
          onToggle={() => onToggle(server)}
          onDelete={() => onDelete(server)}
          onUsage={() => onUsage(server)}
          onUse={() => onUse(server)}
        />
      ))}
    </div>
  );
}

function MCPMarketplaceView({
  workspaceId,
  data,
  onSaved,
}: {
  workspaceId: string;
  data: ReturnType<typeof useMCPWorkspaceSettings>;
  onSaved: (result: MCPRegistrationResult) => void;
}) {
  return (
    <MCPMarketplace
      workspaceId={workspaceId}
      entries={data.marketplace?.entries ?? []}
      stale={data.marketplace?.stale ?? false}
      degraded={data.marketplace?.degraded ?? false}
      loading={data.marketplaceLoading}
      onSearch={async (query) => {
        await data.searchMarketplace(query);
      }}
      onRefresh={async () => {
        await data.refresh();
      }}
      onInstall={async (payload) => {
        onSaved(await data.install(payload));
      }}
    />
  );
}

function MCPViewTabs({
  view,
  onChange,
  marketplaceDisabled,
}: {
  view: "configured" | "marketplace";
  onChange: (view: "configured" | "marketplace") => void;
  marketplaceDisabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2 border-b" role="tablist" aria-label={t("settings:mcpServers")}>
      <Button
        type="button"
        role="tab"
        aria-selected={view === "configured"}
        variant={view === "configured" ? "secondary" : "ghost"}
        className={STANDARD_ACTION_CLASS}
        onClick={() => onChange("configured")}
      >
        {t("settings:mcpConfigured")}
      </Button>
      <Button
        type="button"
        role="tab"
        aria-selected={view === "marketplace"}
        variant={view === "marketplace" ? "secondary" : "ghost"}
        className={STANDARD_ACTION_CLASS}
        onClick={() => onChange("marketplace")}
        disabled={marketplaceDisabled}
      >
        {t("settings:mcpMarketplace")}
      </Button>
    </div>
  );
}

function DeleteMCPServerDialog({
  target,
  deleting,
  onClose,
  onConfirm,
}: {
  target: MCPServerDefinition | null;
  deleting: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={Boolean(target)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:deleteMcpServer")}</DialogTitle>
          <DialogDescription>
            {t("settings:mcpDeleteDescription", { name: target?.display_name ?? "" })}
          </DialogDescription>
        </DialogHeader>
        {selectionImpactTotal(target?.selection_impact) > 0 && (
          <SelectionImpact impact={target?.selection_impact} />
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className={STANDARD_ACTION_CLASS}
            onClick={onClose}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className={STANDARD_ACTION_CLASS}
            onClick={onConfirm}
            disabled={deleting}
          >
            {deleting ? t("settings:deleting") : t("settings:delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MCPUsageEntry({
  workspaceId,
  data,
}: {
  workspaceId: string;
  data: ReturnType<typeof useMCPWorkspaceSettings>;
}) {
  const router = useRouter();
  const params = useSearchParams();
  const usageHint = readMCPContextHint(params, "serverId");
  const usageOpen = params.getAll("view").length === 1 && params.get("view") === "usage";
  const usageServer = data.servers.find(
    (server) => server.id === usageHint.value && server.workspace_id === workspaceId,
  );
  if (!usageOpen) return null;
  const close = () => {
    const next = new URLSearchParams(params);
    next.delete("view");
    next.delete("serverId");
    router.replace(`?${next}`, { scroll: false });
  };
  return (
    <MCPUsage
      workspaceId={workspaceId}
      definitionId={usageHint.value}
      name={usageServer?.display_name}
      invalid={!usageHint.value || (!data.loading && !data.error && !usageServer)}
      onClose={close}
    />
  );
}

export function MCPSettings({ workspaceId }: MCPSettingsProps) {
  return <MCPSettingsContent key={workspaceId} workspaceId={workspaceId} />;
}

function useMCPServerActions(data: ReturnType<typeof useMCPWorkspaceSettings>) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [deleteTarget, setDeleteTarget] = useState<MCPServerDefinition | null>(null);
  const [deleting, setDeleting] = useState(false);
  const toggleServer = async (server: MCPServerDefinition) => {
    try {
      await data.update(server.id, {
        enabled: !server.enabled,
        expected_revision: server.revision,
      });
    } catch {
      toast({ title: t("settings:mcpSaveFailed"), variant: "error" });
    }
  };

  const deleteServer = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await data.remove(deleteTarget, true);
      setDeleteTarget(null);
      if (data.registration.receipt?.definitionId === deleteTarget.id) data.registration.dismiss();
      toast({ title: t("settings:mcpDeleted"), variant: "success" });
    } catch {
      toast({ title: t("settings:mcpDeleteFailed"), variant: "error" });
    } finally {
      setDeleting(false);
    }
  };

  return { deleteTarget, setDeleteTarget, deleting, toggleServer, deleteServer };
}

function MCPSettingsHeader({ onAdd, disabled }: { onAdd: () => void; disabled: boolean }) {
  const { t } = useTranslation();
  return (
    <SettingsPageHeader
      title={t("settings:mcpServers")}
      description={t("settings:workspaceMcpServersDescription")}
      actions={
        <Button
          type="button"
          className={controlSizingClassName("standard", "w-full cursor-pointer md:w-auto")}
          onClick={onAdd}
          disabled={disabled}
        >
          <IconPlus className="mr-2 size-4" />
          {t("settings:addMcpServer")}
        </Button>
      }
    />
  );
}

function CatalogLoadError({ error }: { error: unknown }) {
  const { t } = useTranslation();
  return error ? (
    <p
      className="rounded-md border border-destructive/40 p-3 text-sm text-destructive"
      role="alert"
    >
      {t("settings:mcpLoadFailed")}
    </p>
  ) : null;
}

function MCPSettingsContent({ workspaceId }: MCPSettingsProps) {
  const router = useRouter();
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { toast } = useToast();
  const data = useMCPWorkspaceSettings(workspaceId);
  const [assignment, setAssignment] = useState<MCPRegistrationReceipt | null>(null);
  const [view, setView] = useState<"configured" | "marketplace">("configured");
  const [editor, setEditor] = useState<MCPServerDefinition | null | undefined>(undefined);
  const { deleteTarget, setDeleteTarget, deleting, toggleServer, deleteServer } =
    useMCPServerActions(data);
  const openEditor = (server: MCPServerDefinition | null) => {
    data.registration.dismiss();
    setEditor(server);
  };

  const finishRegistration = (result: MCPRegistrationResult) => {
    if (result.status === "ignored") return;
    setEditor(undefined);
    setView("configured");
  };
  const addDisabled = data.registration.uncertain || data.registration.pending;

  const saveDefinition = async (payload: MCPDefinitionInput, server?: MCPServerDefinition) => {
    if (server) {
      await data.update(server.id, { ...payload, expected_revision: server.revision });
      setEditor(undefined);
      toast({ title: t("settings:mcpSaved"), variant: "success" });
      return;
    }
    const result = await data.create(payload);
    finishRegistration(result);
    if (result.status !== "saved") throw new SettingsSaveCancelledError();
  };

  if (editor !== undefined)
    return (
      <MCPDefinitionForm
        workspaceId={workspaceId}
        server={editor}
        onSave={saveDefinition}
        onClose={() => setEditor(undefined)}
      />
    );

  return (
    <div className="space-y-6 overflow-x-hidden" data-testid="workspace-mcp-settings">
      <MCPSettingsHeader onAdd={() => openEditor(null)} disabled={addDisabled} />
      <MCPViewTabs view={view} onChange={setView} marketplaceDisabled={addDisabled} />
      {data.registration.uncertain && (
        <MCPRegistrationUncertain onReview={data.reviewRegistration} />
      )}
      {view === "configured" && data.registration.receipt && (
        <MCPRegistrationCompletion
          receipt={data.registration.receipt}
          onDone={data.registration.dismiss}
          onUse={() => setAssignment(data.registration.receipt)}
        />
      )}
      <CatalogLoadError error={data.error} />
      {view === "configured" ? (
        <ConfiguredMCPServers
          servers={data.servers}
          loading={data.loading}
          isMobile={isMobile}
          onAdd={() => openEditor(null)}
          addDisabled={addDisabled}
          onUse={(server) =>
            setAssignment({
              workspaceId,
              definitionId: server.id,
              displayName: server.display_name,
            })
          }
          onEdit={openEditor}
          onToggle={(server) => void toggleServer(server)}
          onDelete={setDeleteTarget}
          onUsage={(server) => {
            const href = mcpCatalogUsageHref(workspaceId, server.id);
            if (href) router.push(href, { scroll: false });
          }}
        />
      ) : (
        <MCPMarketplaceView workspaceId={workspaceId} data={data} onSaved={finishRegistration} />
      )}
      {assignment && (
        <MCPAssignmentChooser receipt={assignment} onClose={() => setAssignment(null)} />
      )}
      <MCPUsageEntry workspaceId={workspaceId} data={data} />
      <DeleteMCPServerDialog
        target={deleteTarget}
        deleting={deleting}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => void deleteServer()}
      />
    </div>
  );
}
