"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import {
  IconNetwork,
  IconExternalLink,
  IconCopy,
  IconCheck,
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
import { getBackendConfig } from "@/lib/config";
import { toast } from "sonner";

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

function UrlActions({ url }: { url: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = useCallback(() => {
    navigator.clipboard?.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }, [url]);

  return (
    <div className="flex items-center gap-0.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer h-7 w-7 p-0"
            onClick={handleCopy}
          >
            {copied ? (
              <IconCheck className="h-3.5 w-3.5 text-green-500" />
            ) : (
              <IconCopy className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>Copy URL</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" className="cursor-pointer h-7 w-7 p-0" asChild>
            <a href={url} target="_blank" rel="noopener noreferrer">
              <IconExternalLink className="h-3.5 w-3.5" />
            </a>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Open in new tab</TooltipContent>
      </Tooltip>
    </div>
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
  const Icon = isTunnelActive ? IconPlugConnectedX : IconPlugConnected;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className={`cursor-pointer h-7 w-7 p-0 ${isTunnelActive ? "text-destructive hover:text-destructive" : ""}`}
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
      <TooltipContent>{isTunnelActive ? "Stop tunnel" : "Start tunnel"}</TooltipContent>
    </Tooltip>
  );
}

function PortUrlRow({
  label,
  tip,
  url,
  variant = "outline",
}: {
  label: string;
  tip: string;
  url: string;
  variant?: "outline" | "default";
}) {
  return (
    <div className="flex items-center gap-2 min-w-0">
      <Badge variant={variant} className="text-[10px] px-1.5 py-0 shrink-0">
        {label}
      </Badge>
      <InfoTip text={tip} />
      <span className="text-xs text-muted-foreground truncate min-w-0 flex-1">{url}</span>
      <div className="shrink-0">
        <UrlActions url={url} />
      </div>
    </div>
  );
}

function PortUrlRows({ proxyUrl, tunnelUrl }: { proxyUrl: string; tunnelUrl: string | null }) {
  return (
    <div className="space-y-1 overflow-hidden">
      <PortUrlRow
        label="Proxy"
        tip="Path-based proxy. Works for APIs but may break web apps that expect to be served at /."
        url={proxyUrl}
      />
      {tunnelUrl && (
        <PortUrlRow
          label="Tunnel"
          tip="Dedicated port tunnel. App is served at /, so assets and routing work correctly."
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
  badge: "Detected" | "Manual";
  tunnelPort?: number;
  tunnelPending?: boolean;
  onTunnelStart: (port: number, requestedPort?: number) => void;
  onTunnelStop: (port: number) => void;
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
}: PortRowProps) {
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
      toast.error("Enter a valid port (1-65535) or leave blank for random");
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
            variant={badge === "Detected" ? "secondary" : "outline"}
            className="text-[10px] px-1.5 py-0"
          >
            {badge}
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
            placeholder="Random"
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
            Start
          </Button>
          <InfoTip text="Specify a local port or leave blank for a random one. For Docker/K8s, use a port you've pre-exposed." />
        </div>
      )}

      <PortUrlRows proxyUrl={proxyUrl} tunnelUrl={tunnelUrl} />
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
}) {
  const detectedPortNumbers = new Set(detectedPorts.map((p) => p.port));
  const uniqueManualPorts = manualPorts.filter((p) => !detectedPortNumbers.has(p));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium flex items-center gap-1.5">
          Listening Ports
          <InfoTip text="TCP ports with active listeners inside the remote executor. Click refresh to re-scan." />
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
          Refresh
        </Button>
      </div>

      {!loaded && !loading && (
        <p className="text-xs text-muted-foreground">Click refresh to detect listening ports.</p>
      )}

      {loaded && detectedPorts.length === 0 && !loading && (
        <p className="text-xs text-muted-foreground">No listening ports detected.</p>
      )}

      <div className="space-y-1">
        {detectedPorts.map((p) => (
          <PortRow
            key={`d-${p.port}`}
            port={p.port}
            address={p.address}
            process={p.process}
            sessionId={sessionId}
            badge="Detected"
            tunnelPort={activeTunnels.get(p.port)}
            tunnelPending={pendingTunnels.has(p.port)}
            onTunnelStart={onTunnelStart}
            onTunnelStop={onTunnelStop}
          />
        ))}
        {uniqueManualPorts.map((port) => (
          <PortRow
            key={`m-${port}`}
            port={port}
            sessionId={sessionId}
            badge="Manual"
            tunnelPort={activeTunnels.get(port)}
            tunnelPending={pendingTunnels.has(port)}
            onTunnelStart={onTunnelStart}
            onTunnelStop={onTunnelStop}
          />
        ))}
      </div>
    </div>
  );
}

function ManualPortInput({ onAdd }: { onAdd: (port: number) => void }) {
  const [value, setValue] = useState("");

  const handleAdd = useCallback(() => {
    const port = parseInt(value, 10);
    if (isNaN(port) || port < 1 || port > 65535) {
      toast.error("Enter a valid port (1-65535)");
      return;
    }
    onAdd(port);
    setValue("");
  }, [value, onAdd]);

  return (
    <div className="space-y-2">
      <span className="text-sm font-medium flex items-center gap-1.5">
        Add Port Manually
        <InfoTip text="Add a port that isn't auto-detected. Useful for services not yet started." />
      </span>
      <div className="flex gap-2">
        <Input
          data-testid="port-forward-port-input"
          type="number"
          placeholder="Port number"
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
          Add
        </Button>
      </div>
    </div>
  );
}

function PortForwardDialogContent({
  sessionId,
  activeTunnels,
  setActiveTunnels,
}: {
  sessionId: string;
  activeTunnels: Map<number, number>;
  setActiveTunnels: (updater: (prev: Map<number, number>) => Map<number, number>) => void;
}) {
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
        toast.error("Port already added");
        return;
      }
      setManualPorts((prev) => [...prev, port]);
    },
    [manualPorts],
  );

  return (
    <DialogContent
      data-testid="port-forward-dialog"
      className="sm:max-w-2xl overflow-hidden"
      onOpenAutoFocus={() => !loaded && refresh()}
    >
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconNetwork className="h-5 w-5" />
          Port Forwarding
        </DialogTitle>
      </DialogHeader>
      <div className="space-y-4 min-w-0 max-h-[60vh] overflow-y-auto">
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
        />
        <ManualPortInput onAdd={handleAddManual} />
      </div>
    </DialogContent>
  );
}

export function PortForwardButton({
  isRemoteExecutor,
  sessionId,
  isAgentctlReady,
}: {
  isRemoteExecutor?: boolean;
  sessionId?: string | null;
  isAgentctlReady?: boolean;
}) {
  const [activeTunnels, setActiveTunnelsRaw] = useState<Map<number, number>>(new Map());
  const hasActiveTunnels = activeTunnels.size > 0;

  const setActiveTunnels = useCallback(
    (updater: (prev: Map<number, number>) => Map<number, number>) => {
      setActiveTunnelsRaw((prev) => updater(prev));
    },
    [],
  );

  useEffect(() => {
    if (!sessionId || !isAgentctlReady) return;
    listTunnels(sessionId).then((tunnels) => {
      setActiveTunnelsRaw(new Map(tunnels.map((t) => [t.port, t.tunnel_port])));
    });
  }, [sessionId, isAgentctlReady]);

  if (!isRemoteExecutor || !sessionId || !isAgentctlReady) return null;

  return (
    <Dialog>
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
            ? `Port Forwarding (${activeTunnels.size} tunnel${activeTunnels.size > 1 ? "s" : ""} active)`
            : "Port Forwarding"}
        </TooltipContent>
      </Tooltip>
      <PortForwardDialogContent
        sessionId={sessionId}
        activeTunnels={activeTunnels}
        setActiveTunnels={setActiveTunnels}
      />
    </Dialog>
  );
}
