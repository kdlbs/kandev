"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
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
import { MCPDefinitionForm } from "./mcp-definition-form";
import { MCPMarketplace } from "./mcp-marketplace";

type MCPSettingsProps = { workspaceId: string };

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
            profile: impact?.profile ?? 0,
            repository: impact?.repository ?? 0,
            task: impact?.task ?? 0,
            taskSession: impact?.task_session ?? 0,
          })
        : t("settings:mcpSelectionImpactNone")}
    </p>
  );
}

function ServerCard({
  server,
  onEdit,
  onToggle,
  onDelete,
}: {
  server: MCPServerDefinition;
  onEdit: () => void;
  onToggle: () => void;
  onDelete: () => void;
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
        <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
          <span>{modeLabel(server.execution_mode, t)}</span>
          <span>{transportLabel(server.transport, t)}</span>
          <span>
            {server.source === "registry"
              ? t("settings:mcpSourceRegistry")
              : t("settings:mcpSourceCustom")}
          </span>
        </div>
        <SelectionImpact impact={server.selection_impact} />
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-8"
            onClick={onEdit}
          >
            {t("common:edit")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-8"
            onClick={onToggle}
          >
            {server.enabled ? t("settings:mcpDisable") : t("settings:mcpEnable")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="min-h-11 min-w-11 cursor-pointer text-destructive md:min-h-8 md:min-w-8"
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

function EmptyCatalog({ onAdd }: { onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <IconServer className="mx-auto mb-3 size-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">{t("settings:mcpCatalogEmpty")}</p>
        <Button type="button" className="mt-4 min-h-11 cursor-pointer" onClick={onAdd}>
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
  onEdit,
  onToggle,
  onDelete,
}: {
  servers: MCPServerDefinition[];
  loading: boolean;
  isMobile: boolean;
  onAdd: () => void;
  onEdit: (server: MCPServerDefinition) => void;
  onToggle: (server: MCPServerDefinition) => void;
  onDelete: (server: MCPServerDefinition) => void;
}) {
  if (servers.length === 0 && !loading) return <EmptyCatalog onAdd={onAdd} />;
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
        />
      ))}
    </div>
  );
}

function MCPMarketplaceView({
  data,
  onSaved,
}: {
  data: ReturnType<typeof useMCPWorkspaceSettings>;
  onSaved: () => void;
}) {
  return (
    <MCPMarketplace
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
        await data.install(payload);
        onSaved();
      }}
    />
  );
}

function MCPViewTabs({
  view,
  onChange,
}: {
  view: "configured" | "marketplace";
  onChange: (view: "configured" | "marketplace") => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex gap-2 border-b"
      role="tablist"
      aria-label={t("settings:workspaceMcpServers")}
    >
      <Button
        type="button"
        role="tab"
        aria-selected={view === "configured"}
        variant={view === "configured" ? "secondary" : "ghost"}
        className="min-h-11 cursor-pointer md:min-h-8"
        onClick={() => onChange("configured")}
      >
        {t("settings:mcpConfigured")}
      </Button>
      <Button
        type="button"
        role="tab"
        aria-selected={view === "marketplace"}
        variant={view === "marketplace" ? "secondary" : "ghost"}
        className="min-h-11 cursor-pointer md:min-h-8"
        onClick={() => onChange("marketplace")}
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
            className="min-h-11 cursor-pointer"
            onClick={onClose}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="min-h-11 cursor-pointer"
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

export function MCPSettings({ workspaceId }: MCPSettingsProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { toast } = useToast();
  const data = useMCPWorkspaceSettings(workspaceId);
  const [view, setView] = useState<"configured" | "marketplace">("configured");
  const [editor, setEditor] = useState<MCPServerDefinition | null | undefined>(undefined);
  const [deleteTarget, setDeleteTarget] = useState<MCPServerDefinition | null>(null);
  const [deleting, setDeleting] = useState(false);

  const saveDefinition = async (payload: MCPDefinitionInput, server?: MCPServerDefinition) => {
    if (server) await data.update(server.id, { ...payload, expected_revision: server.revision });
    else await data.create(payload);
    setEditor(undefined);
    toast({ title: t("settings:mcpSaved"), variant: "success" });
  };

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
      toast({ title: t("settings:mcpDeleted"), variant: "success" });
    } catch {
      toast({ title: t("settings:mcpDeleteFailed"), variant: "error" });
    } finally {
      setDeleting(false);
    }
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
      <SettingsPageHeader
        title={t("settings:workspaceMcpServers")}
        description={t("settings:workspaceMcpServersDescription")}
        actions={
          <Button
            type="button"
            className="min-h-11 w-full cursor-pointer md:min-h-8 md:w-auto"
            onClick={() => setEditor(null)}
          >
            <IconPlus className="mr-2 size-4" />
            {t("settings:addMcpServer")}
          </Button>
        }
      />
      <MCPViewTabs view={view} onChange={setView} />
      {Boolean(data.error) && (
        <p
          className="rounded-md border border-destructive/40 p-3 text-sm text-destructive"
          role="alert"
        >
          {t("settings:mcpLoadFailed")}
        </p>
      )}
      {view === "configured" ? (
        <ConfiguredMCPServers
          servers={data.servers}
          loading={data.loading}
          isMobile={isMobile}
          onAdd={() => setEditor(null)}
          onEdit={setEditor}
          onToggle={(server) => void toggleServer(server)}
          onDelete={setDeleteTarget}
        />
      ) : (
        <MCPMarketplaceView
          data={data}
          onSaved={() => toast({ title: t("settings:mcpSaved"), variant: "success" })}
        />
      )}
      <DeleteMCPServerDialog
        target={deleteTarget}
        deleting={deleting}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => void deleteServer()}
      />
    </div>
  );
}
