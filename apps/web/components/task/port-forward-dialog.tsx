"use client";

import { useState, useCallback } from "react";
import {
  IconNetwork,
  IconExternalLink,
  IconCopy,
  IconCheck,
  IconRefresh,
  IconPlus,
  IconLoader2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { listPorts, type ListeningPort } from "@/lib/api/domains/port-api";
import { getBackendConfig } from "@/lib/config";
import { toast } from "sonner";

function buildPortProxyUrl(sessionId: string, port: number): string {
  const backendUrl = getBackendConfig().apiBaseUrl;
  return `${backendUrl}/port-proxy/${sessionId}/${port}/`;
}

function CopyUrlButton({ url }: { url: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard?.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }, [url]);

  return (
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
  );
}

type PortRowProps = {
  port: number;
  address?: string;
  sessionId: string;
  badge: "Detected" | "Manual";
};

function PortRow({ port, address, sessionId, badge }: PortRowProps) {
  const proxyUrl = buildPortProxyUrl(sessionId, port);

  return (
    <div
      data-testid={`port-forward-row-${port}`}
      className="flex items-center justify-between gap-2 px-3 py-2 rounded-md bg-muted/40 hover:bg-muted/60 transition-colors"
    >
      <div className="flex items-center gap-2 min-w-0">
        <span className="text-sm font-mono font-medium">{port}</span>
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
        <CopyUrlButton url={proxyUrl} />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="ghost" className="cursor-pointer h-7 w-7 p-0" asChild>
              <a href={proxyUrl} target="_blank" rel="noopener noreferrer">
                <IconExternalLink className="h-3.5 w-3.5" />
              </a>
            </Button>
          </TooltipTrigger>
          <TooltipContent>Open in new tab</TooltipContent>
        </Tooltip>
      </div>
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
}: {
  detectedPorts: ListeningPort[];
  manualPorts: number[];
  sessionId: string;
  loading: boolean;
  loaded: boolean;
  onRefresh: () => void;
}) {
  const detectedPortNumbers = new Set(detectedPorts.map((p) => p.port));
  const uniqueManualPorts = manualPorts.filter((p) => !detectedPortNumbers.has(p));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Listening Ports</span>
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
            sessionId={sessionId}
            badge="Detected"
          />
        ))}
        {uniqueManualPorts.map((port) => (
          <PortRow key={`m-${port}`} port={port} sessionId={sessionId} badge="Manual" />
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
      <span className="text-sm font-medium">Add Port Manually</span>
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

function PortForwardDialogContent({ sessionId }: { sessionId: string }) {
  const [detectedPorts, setDetectedPorts] = useState<ListeningPort[]>([]);
  const [manualPorts, setManualPorts] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const ports = await listPorts(sessionId);
      setDetectedPorts(ports);
      setLoaded(true);
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
      className="sm:max-w-md"
      onOpenAutoFocus={() => !loaded && refresh()}
    >
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconNetwork className="h-5 w-5" />
          Port Forwarding
        </DialogTitle>
      </DialogHeader>
      <div className="space-y-4">
        <PortListSection
          detectedPorts={detectedPorts}
          manualPorts={manualPorts}
          sessionId={sessionId}
          loading={loading}
          loaded={loaded}
          onRefresh={refresh}
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
  if (!isRemoteExecutor || !sessionId || !isAgentctlReady) return null;

  return (
    <Dialog>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>
            <Button
              data-testid="port-forward-button"
              size="sm"
              variant="outline"
              className="cursor-pointer px-2"
            >
              <IconNetwork className="h-4 w-4" />
            </Button>
          </DialogTrigger>
        </TooltipTrigger>
        <TooltipContent>Port Forwarding</TooltipContent>
      </Tooltip>
      <PortForwardDialogContent sessionId={sessionId} />
    </Dialog>
  );
}
