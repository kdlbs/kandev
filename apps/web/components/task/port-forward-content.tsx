import { IconPlus } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { PortListSection, InfoTip } from "./port-forward-list";
import { usePortForwardingVisibility } from "./port-forwarding-visibility-provider";
import type { PortForwardManagement } from "./use-port-forward-management";

export function PortForwardContent({
  sessionId,
  management,
  isMobile,
  onOpenBrowserPanel,
}: {
  sessionId: string;
  management: PortForwardManagement;
  isMobile: boolean;
  onOpenBrowserPanel?: (url: string) => void;
}) {
  return (
    <div
      data-testid="port-forward-scroll-body"
      className="min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 pb-[calc(1rem+env(safe-area-inset-bottom))] md:max-h-[60dvh] md:px-0 md:pb-0"
    >
      <PortListSection
        detectedPorts={management.detectedPorts}
        manualPorts={management.manualPorts}
        sessionId={sessionId}
        loading={management.loading}
        loaded={management.loaded}
        onRefresh={management.refresh}
        activeTunnels={management.activeTunnels}
        pendingTunnels={management.pendingTunnels}
        onTunnelStart={management.handleTunnelStart}
        onTunnelStop={management.handleTunnelStop}
        onOpenBrowserPanel={onOpenBrowserPanel}
      />
      <ManualPortInput management={management} />
      {isMobile && <HeaderShortcutPreference />}
    </div>
  );
}

function ManualPortInput({ management }: { management: PortForwardManagement }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <label
        htmlFor="port-forward-port-input"
        className="text-sm font-medium flex items-center gap-1.5"
      >
        {t("task:addPortManually")}
        <InfoTip text={t("task:addAPortThatIsnT")} />
      </label>
      <div className="flex gap-2">
        <Input
          id="port-forward-port-input"
          data-testid="port-forward-port-input"
          type="number"
          inputMode="numeric"
          placeholder={t("task:portNumber")}
          value={management.manualValue}
          onChange={(event) => management.setManualValue(event.target.value)}
          onKeyDown={(event) =>
            event.key === "Enter" && (event.preventDefault(), management.addManualPort())
          }
          className="min-w-0"
          min={1}
          max={65535}
        />
        <Button
          variant="outline"
          data-testid="port-forward-add-button"
          className="cursor-pointer gap-1 shrink-0"
          onClick={management.addManualPort}
        >
          <IconPlus className="h-3.5 w-3.5" />
          {t("task:add")}
        </Button>
      </div>
    </div>
  );
}

function HeaderShortcutPreference() {
  const { t } = useTranslation();
  const { enabled, isUpdating, canToggle, togglePortForwarding } = usePortForwardingVisibility();
  return (
    <div className="border-t pt-3">
      <div className="flex min-h-11 items-center justify-between gap-3">
        <label
          htmlFor="port-forward-header-shortcut"
          className="min-w-0 cursor-pointer text-sm font-medium"
        >
          {t("task:portForwardingShowInHeader")}
        </label>
        <Switch
          id="port-forward-header-shortcut"
          data-testid="port-forward-header-shortcut"
          className="after:-inset-y-[15px] shrink-0 cursor-pointer"
          checked={enabled}
          disabled={!canToggle || isUpdating}
          onCheckedChange={() => void togglePortForwarding({ preserveDialogOpen: true })}
          aria-describedby="port-forward-header-shortcut-description"
        />
      </div>
      <p id="port-forward-header-shortcut-description" className="text-xs text-muted-foreground">
        {t("task:portForwardingHeaderShortcutDescription")}
      </p>
    </div>
  );
}
