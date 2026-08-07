"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import {
  IconNetwork,
  IconRefresh,
  IconPlus,
  IconLoader2,
  IconPlugConnected,
  IconPlugConnectedX,
  IconInfoCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { listPorts, listTunnels, type ListeningPort } from "@/lib/api/domains/port-api";
import { useTunnelActions } from "./use-tunnel-actions";
import { PortUrlActions } from "./port-forward-dialog-actions";
import { getBackendConfig } from "@/lib/config";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { usePortForwardingVisibility } from "./port-forwarding-visibility-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";

function buildPortProxyUrl(sessionId: string, port: number): string {
  const backendUrl = getBackendConfig().apiBaseUrl;
  return `${backendUrl}/port-proxy/${sessionId}/${port}/`;
}

function buildTunnelUrl(tunnelPort: number): string {
  const backendUrl = getBackendConfig().apiBaseUrl;
  const { protocol, hostname } = new URL(backendUrl);
  return `${protocol}//${hostname}:${tunnelPort}/`;
}

function InfoTip({ text }: { text: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/60 shrink-0 cursor-help" />
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-[240px] text-xs">
        {text}
      </TooltipContent>
    </Tooltip>
  );
}

function TunnelToggleButton({
  isTunnelActive,
  tunnelPending,
  onStop,
  onToggleForm,
}: {
  isTunnelActive: boolean;
  tunnelPending?: boolean;
  onStop: () => void;
  onToggleForm: () => void;
}) {
  const { t } = useTranslation();
  const Icon = isTunnelActive ? IconPlugConnectedX : IconPlugConnected;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          aria-label={isTunnelActive ? t("task:stopTunnel") : t("task:startTunnel")}
          className={`cursor-pointer min-h-11 min-w-11 p-0 sm:h-7 sm:w-7 sm:min-h-0 sm:min-w-0 ${isTunnelActive ? "text-destructive hover:text-destructive" : ""}`}
          onClick={isTunnelActive ? onStop : onToggleForm}
          disabled={tunnelPending}
        >
          {tunnelPending ? (
            <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Icon className="h-3.5 w-3.5" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {isTunnelActive ? t("task:stopTunnel") : t("task:startTunnel")}
      </TooltipContent>
    </Tooltip>
  );
}

function PortUrlRow({
  label,
  tip,
  url,
  variant = "outline",
  onOpenBrowserPanel,
  browserActionTestId,
}: {
  label: string;
  tip: string;
  url: string;
  variant?: "outline" | "default";
  onOpenBrowserPanel?: (url: string) => void;
  browserActionTestId?: string;
}) {
  return (
    <div className="flex items-center gap-2 min-w-0">
      <Badge variant={variant} className="text-[10px] px-1.5 py-0 shrink-0">
        {label}
      </Badge>
      <InfoTip text={tip} />
      <span className="text-xs text-muted-foreground truncate min-w-0 flex-1">{url}</span>
      <div className="shrink-0">
        <PortUrlActions
          url={url}
          onOpenBrowserPanel={onOpenBrowserPanel}
          browserActionTestId={browserActionTestId}
        />
      </div>
    </div>
  );
}

function PortUrlRows({
  proxyUrl,
  tunnelUrl,
  onOpenBrowserPanel,
  browserActionTestId,
}: {
  proxyUrl: string;
  tunnelUrl: string | null;
  onOpenBrowserPanel?: (url: string) => void;
  browserActionTestId: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 overflow-hidden">
      <PortUrlRow
        label={t("task:proxy")}
        tip={t("task:pathBasedProxyWorksForApis")}
        url={proxyUrl}
        onOpenBrowserPanel={onOpenBrowserPanel}
        browserActionTestId={browserActionTestId}
      />
      {tunnelUrl && (
        <PortUrlRow
          label={t("task:tunnel")}
          tip={t("task:dedicatedPortTunnelAppIsServed")}
          url={tunnelUrl}
          variant="default"
        />
      )}
    </div>
  );
}

type PortRowProps = {
  port: number;
  address?: string;
  process?: string;
  sessionId: string;
  badge: "detected" | "manual";
  tunnelPort?: number;
  tunnelPending?: boolean;
  onTunnelStart: (port: number, requestedPort?: number) => void;
  onTunnelStop: (port: number) => void;
  onOpenBrowserPanel?: (url: string) => void;
};

function PortRow({
  port,
  address,
  process,
  sessionId,
  badge,
  tunnelPort,
  tunnelPending,
  onTunnelStart,
  onTunnelStop,
  onOpenBrowserPanel,
}: PortRowProps) {
  const { t } = useTranslation();
  const [showTunnelForm, setShowTunnelForm] = useState(false);
  const [tunnelPortInput, setTunnelPortInput] = useState("");
  const proxyUrl = buildPortProxyUrl(sessionId, port);
  const tunnelUrl = tunnelPort ? buildTunnelUrl(tunnelPort) : null;
  const isTunnelActive = !!tunnelPort;

  const handleStartTunnel = useCallback(() => {
    const requestedPort = tunnelPortInput ? parseInt(tunnelPortInput, 10) : undefined;
    if (
      tunnelPortInput &&
      (isNaN(requestedPort!) || requestedPort! < 1 || requestedPort! > 65535)
    ) {
      toast.error(t("task:enterAValidPort165535"));
      return;
    }
    onTunnelStart(port, requestedPort);
    setShowTunnelForm(false);
    setTunnelPortInput("");
  }, [port, tunnelPortInput, onTunnelStart]);

  return (
    <div
      data-testid={`port-forward-row-${port}`}
      className="rounded-md bg-muted/40 hover:bg-muted/60 transition-colors px-3 py-2 space-y-1.5"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-sm font-mono font-medium">{port}</span>
          {process && (
            <span className="text-xs text-muted-foreground truncate max-w-[120px]">{process}</span>
          )}
          {address && address !== "0.0.0.0" && address !== "*" && (
            <span className="text-xs text-muted-foreground">{address}</span>
          )}
          <Badge
            variant={badge === "detected" ? "secondary" : "outline"}
            className="text-[10px] px-1.5 py-0"
          >
            {badge === "detected" ? t("task:detected") : t("task:manual")}
          </Badge>
        </div>
        <div className="flex items-center gap-0.5">
          <TunnelToggleButton
            isTunnelActive={isTunnelActive}
            tunnelPending={tunnelPending}
            onStop={() => onTunnelStop(port)}
            onToggleForm={() => setShowTunnelForm((v) => !v)}
          />
        </div>
      </div>

      {showTunnelForm && !isTunnelActive && (
        <div className="flex items-center gap-2">
          <Input
            type="number"
            placeholder={t("task:random")}
            value={tunnelPortInput}
            onChange={(e) => setTunnelPortInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), handleStartTunnel())}
            className="h-7 text-xs w-24"
            min={1}
            max={65535}
          />
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer h-7 text-xs gap-1"
            onClick={handleStartTunnel}
            disabled={tunnelPending}
          >
            {t("task:start2")}
          </Button>
          <InfoTip text={t("task:specifyALocalPortOrLeave")} />
        </div>
      )}

      <PortUrlRows
        proxyUrl={proxyUrl}
        tunnelUrl={tunnelUrl}
        onOpenBrowserPanel={onOpenBrowserPanel}
        browserActionTestId={`port-forward-open-browser-${port}`}
      />
    </div>
  );
}

function PortListSection({
  detectedPorts,
  manualPorts,
  sessionId,
  loading,
  loaded,
  onRefresh,
  activeTunnels,
  pendingTunnels,
  onTunnelStart,
  onTunnelStop,
  onOpenBrowserPanel,
}: {
  detectedPorts: ListeningPort[];
  manualPorts: number[];
  sessionId: string;
  loading: boolean;
  loaded: boolean;
  onRefresh: () => void;
  activeTunnels: Map<number, number>;
  pendingTunnels: Set<number>;
  onTunnelStart: (port: number, requestedPort?: number) => void;
  onTunnelStop: (port: number) => void;
  onOpenBrowserPanel?: (url: string) => void;
}) {
  const { t } = useTranslation();
  const detectedPortNumbers = new Set(detectedPorts.map((p) => p.port));
  const uniqueManualPorts = manualPorts.filter((p) => !detectedPortNumbers.has(p));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium flex items-center gap-1.5">
          {t("task:listeningPorts")}
          <InfoTip text={t("task:tcpPortsWithActiveListenersInside")} />
        </span>
        <Button
          size="sm"
          variant="ghost"
          data-testid="port-forward-refresh"
          className="cursor-pointer h-7 gap-1 text-xs"
          onClick={onRefresh}
          disabled={loading}
        >
          {loading ? (
            <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <IconRefresh className="h-3.5 w-3.5" />
          )}
          {t("task:refresh")}
        </Button>
      </div>

      {!loaded && !loading && (
        <p className="text-xs text-muted-foreground">
          {t("task:clickRefreshToDetectListeningPorts")}
        </p>
      )}

      {loaded && detectedPorts.length === 0 && !loading && (
        <p className="text-xs text-muted-foreground">{t("task:noListeningPortsDetected")}</p>
      )}

      <div className="space-y-1">
        {detectedPorts.map((p) => (
          <PortRow
            key={`d-${p.port}`}
            port={p.port}
            address={p.address}
            process={p.process}
            sessionId={sessionId}
            badge="detected"
            tunnelPort={activeTunnels.get(p.port)}
            tunnelPending={pendingTunnels.has(p.port)}
            onTunnelStart={onTunnelStart}
            onTunnelStop={onTunnelStop}
            onOpenBrowserPanel={onOpenBrowserPanel}
          />
        ))}
        {uniqueManualPorts.map((port) => (
          <PortRow
            key={`m-${port}`}
            port={port}
            sessionId={sessionId}
            badge="manual"
            tunnelPort={activeTunnels.get(port)}
            tunnelPending={pendingTunnels.has(port)}
            onTunnelStart={onTunnelStart}
            onTunnelStop={onTunnelStop}
            onOpenBrowserPanel={onOpenBrowserPanel}
          />
        ))}
      </div>
    </div>
  );
}

function ManualPortInput({ onAdd }: { onAdd: (port: number) => void }) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");

  const handleAdd = useCallback(() => {
    const port = parseInt(value, 10);
    if (isNaN(port) || port < 1 || port > 65535) {
      toast.error(t("task:enterAValidPort1655352"));
      return;
    }
    onAdd(port);
    setValue("");
  }, [value, onAdd]);

  return (
    <div className="space-y-2">
      <span className="text-sm font-medium flex items-center gap-1.5">
        {t("task:addPortManually")}
        <InfoTip text={t("task:addAPortThatIsnT")} />
      </span>
      <div className="flex gap-2">
        <Input
          data-testid="port-forward-port-input"
          type="number"
          placeholder={t("task:portNumber")}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), handleAdd())}
          className="h-8"
          min={1}
          max={65535}
        />
        <Button
          size="sm"
          variant="outline"
          data-testid="port-forward-add-button"
          className="cursor-pointer h-8 gap-1"
          onClick={handleAdd}
        >
          <IconPlus className="h-3.5 w-3.5" />
          {t("task:add")}
        </Button>
      </div>
    </div>
  );
}

function PortForwardDialogContent({
  sessionId,
  activeTunnels,
  setActiveTunnels,
  onOpenBrowserPanel,
}: {
  sessionId: string;
  activeTunnels: Map<number, number>;
  setActiveTunnels: (updater: (prev: Map<number, number>) => Map<number, number>) => void;
  onOpenBrowserPanel?: (url: string) => void;
}) {
  const { t } = useTranslation();
  const [detectedPorts, setDetectedPorts] = useState<ListeningPort[]>([]);
  const [manualPorts, setManualPorts] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const { pendingTunnels, handleTunnelStart, handleTunnelStop } = useTunnelActions(
    sessionId,
    setActiveTunnels,
  );

  // Use a ref so refresh doesn't depend on activeTunnels identity (Map reference
  // changes when listTunnels resolves, which would recreate the callback).
  const activeTunnelsRef = useRef(activeTunnels);
  activeTunnelsRef.current = activeTunnels;

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const ports = await listPorts(sessionId);
      setDetectedPorts(ports);
      setLoaded(true);

      const tunnels = activeTunnelsRef.current;
      const detectedSet = new Set(ports.map((p) => p.port));
      const tunnelOnlyPorts = [...tunnels.keys()].filter((p) => !detectedSet.has(p));
      if (tunnelOnlyPorts.length > 0) {
        setManualPorts((prev) => {
          const existing = new Set(prev);
          const toAdd = tunnelOnlyPorts.filter((p) => !existing.has(p));
          return toAdd.length > 0 ? [...prev, ...toAdd] : prev;
        });
      }
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  const handleAddManual = useCallback(
    (port: number) => {
      if (manualPorts.includes(port)) {
        toast.error(t("task:portAlreadyAdded"));
        return;
      }
      setManualPorts((prev) => [...prev, port]);
    },
    [manualPorts],
  );

  return (
    <DialogContent
      data-testid="port-forward-dialog"
      className="max-h-[calc(100dvh-2rem)] overflow-hidden sm:max-w-2xl"
      onOpenAutoFocus={() => !loaded && refresh()}
    >
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconNetwork className="h-5 w-5" />
          {t("task:portForwarding")}
        </DialogTitle>
      </DialogHeader>
      <div className="min-w-0 max-h-[calc(100dvh-8rem)] space-y-4 overflow-y-auto overscroll-contain sm:max-h-[60vh]">
        <PortListSection
          detectedPorts={detectedPorts}
          manualPorts={manualPorts}
          sessionId={sessionId}
          loading={loading}
          loaded={loaded}
          onRefresh={refresh}
          activeTunnels={activeTunnels}
          pendingTunnels={pendingTunnels}
          onTunnelStart={handleTunnelStart}
          onTunnelStop={handleTunnelStop}
          onOpenBrowserPanel={onOpenBrowserPanel}
        />
        <ManualPortInput onAdd={handleAddManual} />
      </div>
    </DialogContent>
  );
}

export function PortForwardButton({ sessionId }: { sessionId?: string | null }) {
  const { t } = useTranslation();
  const { enabled, canToggle, dialogOpen, setDialogOpen } = usePortForwardingVisibility();
  const openBrowserPanel = useDockviewStore((state) =>
    state.api ? state.openBrowserPanel : undefined,
  );
  const [activeTunnels, setActiveTunnelsRaw] = useState<Map<number, number>>(new Map());
  const hasActiveTunnels = activeTunnels.size > 0;
  const handleOpenBrowserPanel = useCallback(
    (url: string) => {
      if (!openBrowserPanel) return;
      openBrowserPanel(url);
      setDialogOpen(false);
    },
    [openBrowserPanel, setDialogOpen],
  );

  const setActiveTunnels = useCallback(
    (updater: (prev: Map<number, number>) => Map<number, number>) => {
      setActiveTunnelsRaw((prev) => updater(prev));
    },
    [],
  );

  useEffect(() => {
    if (!sessionId || !canToggle) {
      setActiveTunnelsRaw(new Map());
      return;
    }
    let cancelled = false;
    listTunnels(sessionId).then((tunnels) => {
      if (cancelled) return;
      setActiveTunnelsRaw(new Map(tunnels.map((t) => [t.port, t.tunnel_port])));
    });
    return () => {
      cancelled = true;
    };
  }, [canToggle, sessionId]);

  if (!enabled || !canToggle || !sessionId) return null;

  return (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>
            <Button
              data-testid="port-forward-button"
              size="sm"
              variant={hasActiveTunnels ? "default" : "outline"}
              className="cursor-pointer px-2"
            >
              <IconNetwork className="h-4 w-4" />
            </Button>
          </DialogTrigger>
        </TooltipTrigger>
        <TooltipContent>
          {hasActiveTunnels
            ? t("task:portForwardingTunnelActive", { count: activeTunnels.size })
            : t("task:portForwarding")}
        </TooltipContent>
      </Tooltip>
      <PortForwardDialogContent
        sessionId={sessionId}
        activeTunnels={activeTunnels}
        setActiveTunnels={setActiveTunnels}
        onOpenBrowserPanel={openBrowserPanel ? handleOpenBrowserPanel : undefined}
      />
    </Dialog>
  );
}
