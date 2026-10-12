"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { IconNetwork, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { listPorts, listTunnels, type ListeningPort } from "@/lib/api/domains/port-api";
import { useTunnelActions } from "./use-tunnel-actions";
import { PortListSection, InfoTip } from "./port-forward-list";
import { toast } from "@/lib/toast/sonner";
import { useTranslation } from "react-i18next";
import { usePortForwardingVisibility } from "./port-forwarding-visibility-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";

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
          className="min-w-0"
          min={1}
          max={65535}
        />
        <Button
          size="default"
          variant="outline"
          data-testid="port-forward-add-button"
          className="cursor-pointer gap-1"
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

  useEffect(() => {
    setManualPorts((previous) => {
      const added = [...activeTunnels.keys()].filter((port) => !previous.includes(port));
      return added.length ? [...previous, ...added] : previous;
    });
  }, [activeTunnels]);

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
      className="max-h-[calc(100dvh-2rem)] overflow-hidden md:max-w-2xl [&>[data-slot=dialog-close]]:max-md:size-11 [&>[data-slot=dialog-close]]:[@media(pointer:coarse)]:size-11"
      onOpenAutoFocus={() => !loaded && refresh()}
    >
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconNetwork className="h-5 w-5" />
          {t("task:portForwarding")}
        </DialogTitle>
      </DialogHeader>
      <div
        data-testid="port-forward-scroll-body"
        className="min-h-0 min-w-0 max-h-[calc(100dvh-8rem)] space-y-4 overflow-y-auto overscroll-contain pb-[env(safe-area-inset-bottom)] md:max-h-[60dvh]"
      >
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
  const { enabled, canToggle } = usePortForwardingVisibility();
  if (!enabled || !canToggle || !sessionId) return null;
  return <SessionPortForwardControl key={sessionId} sessionId={sessionId} />;
}

function SessionPortForwardControl({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const { dialogOpen, setDialogOpen } = usePortForwardingVisibility();
  const openBrowserPanel = useDockviewStore((state) =>
    state.api ? state.openBrowserPanel : undefined,
  );
  const mutatedPorts = useRef(new Set<number>());
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
      setActiveTunnelsRaw((previous) => {
        const next = updater(previous);
        for (const port of new Set([...previous.keys(), ...next.keys()])) {
          if (previous.get(port) !== next.get(port)) mutatedPorts.current.add(port);
        }
        return next;
      });
    },
    [],
  );

  useEffect(() => {
    let cancelled = false;
    listTunnels(sessionId).then((tunnels) => {
      if (cancelled) return;
      setActiveTunnelsRaw((previous) => {
        const hydrated = new Map(tunnels.map((t) => [t.port, t.tunnel_port]));
        for (const port of mutatedPorts.current) {
          const tunnelPort = previous.get(port);
          if (tunnelPort === undefined) hydrated.delete(port);
          else hydrated.set(port, tunnelPort);
        }
        return hydrated;
      });
    });
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  return (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <DialogTrigger asChild>
            <Button
              data-testid="port-forward-button"
              size="default"
              variant={hasActiveTunnels ? "default" : "outline"}
              aria-label={t("task:portForwarding")}
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
