"use client";
import { useState, useCallback, useRef, type ReactNode, type RefObject } from "react";
import {
  IconRefresh,
  IconLoader2,
  IconPlugConnected,
  IconPlugConnectedX,
  IconInfoCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ListeningPort } from "@/lib/api/domains/port-api";
import { PortUrlActions } from "./port-forward-dialog-actions";
import { buildPortForwardRows, type ForwardablePort } from "./port-forward-rows";
import { getBackendConfig } from "@/lib/config";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";

function buildPortProxyUrl(sessionId: string, port: number): string {
  const backendUrl = getBackendConfig().apiBaseUrl;
  return `${backendUrl}/port-proxy/${sessionId}/${port}/`;
}

function buildTunnelUrl(tunnelPort: number): string {
  const backendUrl = getBackendConfig().apiBaseUrl;
  const { protocol, hostname } = new URL(backendUrl);
  return `${protocol}//${hostname}:${tunnelPort}/`;
}

export function InfoTip({ text }: { text: string }) {
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
  buttonRef,
  port,
}: {
  isTunnelActive: boolean;
  tunnelPending?: boolean;
  onStop: () => void;
  onToggleForm: () => void;
  buttonRef: RefObject<HTMLButtonElement | null>;
  port: number;
}) {
  const { t } = useTranslation();
  const Icon = isTunnelActive ? IconPlugConnectedX : IconPlugConnected;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="icon"
          ref={buttonRef}
          data-testid={`port-forward-tunnel-toggle-${port}`}
          variant="ghost"
          aria-label={isTunnelActive ? t("task:stopTunnel") : t("task:startTunnel")}
          className={`cursor-pointer shrink-0 ${isTunnelActive ? "text-destructive hover:text-destructive" : ""}`}
          onClick={() => {
            if (!tunnelPending) (isTunnelActive ? onStop : onToggleForm)();
          }}
          aria-disabled={tunnelPending}
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
    <div className="grid grid-cols-[auto_1fr] items-center gap-2 min-w-0 md:flex">
      <Badge variant={variant} className="text-[10px] px-1.5 py-0 shrink-0">
        {label}
      </Badge>
      <InfoTip text={tip} />
      <span className="col-span-2 text-xs font-mono text-muted-foreground break-all min-w-0 flex-1 md:truncate">
        {url}
      </span>
      <div className="col-span-2 shrink-0">
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
      {tunnelUrl && (
        <PortUrlRow
          label={t("task:tunnel")}
          tip={t("task:dedicatedPortTunnelAppIsServed")}
          url={tunnelUrl}
          variant="default"
        />
      )}
      <PortUrlRow
        label={t("task:proxy")}
        tip={t("task:pathBasedProxyWorksForApis")}
        url={proxyUrl}
        onOpenBrowserPanel={onOpenBrowserPanel}
        browserActionTestId={browserActionTestId}
      />
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

function TunnelPortForm({
  value,
  onChange,
  onStart,
  pending,
}: {
  value: string;
  onChange: (value: string) => void;
  onStart: () => void;
  pending?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <Input
        type="number"
        placeholder={t("task:random")}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), onStart())}
        className="text-xs w-24"
        min={1}
        max={65535}
      />
      <Button
        size="default"
        variant="outline"
        className="cursor-pointer text-xs gap-1"
        onClick={onStart}
        disabled={pending}
      >
        {t("task:start2")}
      </Button>
      <InfoTip text={t("task:specifyALocalPortOrLeave")} />
    </div>
  );
}

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
  const toggleRef = useRef<HTMLButtonElement>(null);
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
    toggleRef.current?.focus();
    setShowTunnelForm(false);
    setTunnelPortInput("");
  }, [port, tunnelPortInput, onTunnelStart, t]);

  return (
    <div
      data-testid={`port-forward-row-${port}`}
      data-forwarded={isTunnelActive}
      className="rounded-md bg-muted/40 hover:bg-muted/60 transition-colors px-3 py-2 space-y-1.5"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 min-w-0">
          <span className="text-sm font-mono font-medium">{port}</span>
          {process && (
            <span className="text-xs text-muted-foreground truncate max-w-[120px]">{process}</span>
          )}
          {address && address !== "0.0.0.0" && address !== "*" && (
            <span className="text-xs text-muted-foreground truncate max-w-[120px]">{address}</span>
          )}
          <Badge
            variant={badge === "detected" ? "secondary" : "outline"}
            className="text-[10px] px-1.5 py-0"
          >
            {badge === "detected" ? t("task:detected") : t("task:manual")}
          </Badge>
          {isTunnelActive && (
            <span className="text-xs font-medium text-primary flex items-center gap-1">
              <IconPlugConnected className="h-3.5 w-3.5" />
              {t("task:forwarding")}
            </span>
          )}
        </div>
        <div className="flex items-center gap-0.5">
          <TunnelToggleButton
            port={port}
            buttonRef={toggleRef}
            isTunnelActive={isTunnelActive}
            tunnelPending={tunnelPending}
            onStop={() => onTunnelStop(port)}
            onToggleForm={() => setShowTunnelForm((v) => !v)}
          />
        </div>
      </div>

      {showTunnelForm && !isTunnelActive && (
        <TunnelPortForm
          value={tunnelPortInput}
          onChange={setTunnelPortInput}
          onStart={handleStartTunnel}
          pending={tunnelPending}
        />
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

export function PortListSection({
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
  const rows = buildPortForwardRows(detectedPorts, manualPorts, activeTunnels);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium flex items-center gap-1.5">
          {t("task:listeningPorts")}
          <InfoTip text={t("task:tcpPortsWithActiveListenersInside")} />
        </span>
        <Button
          size="default"
          variant="ghost"
          data-testid="port-forward-refresh"
          className="cursor-pointer gap-1 text-xs"
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

      <PortRows
        rows={rows}
        sessionId={sessionId}
        pendingTunnels={pendingTunnels}
        onTunnelStart={onTunnelStart}
        onTunnelStop={onTunnelStop}
        onOpenBrowserPanel={onOpenBrowserPanel}
      />
    </div>
  );
}

function PortRows({
  rows,
  pendingTunnels,
  ...actions
}: {
  rows: ForwardablePort[];
  pendingTunnels: Set<number>;
} & Pick<PortRowProps, "sessionId" | "onTunnelStart" | "onTunnelStop" | "onOpenBrowserPanel">) {
  const { t } = useTranslation();
  const activeCount = rows.filter((row) => row.tunnelPort !== undefined).length;
  const children: ReactNode[] = [];
  if (activeCount > 0)
    children.push(
      <h3
        key="forwarded"
        className="flex items-center gap-2 text-sm font-medium pt-1"
        data-testid="port-forward-active-heading"
      >
        {t("task:forwardedPorts")}
        <Badge variant="secondary" className="tabular-nums">
          {activeCount}
        </Badge>
      </h3>,
    );
  for (const [index, row] of rows.entries()) {
    if (index === activeCount)
      children.push(
        <h3 key="other" className="text-sm font-medium pt-3">
          {t("task:otherPorts")}
        </h3>,
      );
    children.push(
      <PortRow key={row.port} {...row} {...actions} tunnelPending={pendingTunnels.has(row.port)} />,
    );
  }
  return <div className="space-y-2">{children}</div>;
}
