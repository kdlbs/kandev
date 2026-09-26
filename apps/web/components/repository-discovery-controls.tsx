"use client";

import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
import { useDiscoveryRootActions } from "@/hooks/domains/workspace/use-discovery-root-actions";
import { RepositoryDiscoveryRootControls } from "@/components/repository-discovery-root-controls";
import { cn } from "@/lib/utils";

type RepositoryDiscoveryControlsProps = {
  workspaceId: string | null;
  enabled?: boolean;
  className?: string;
  presentation?: "card" | "picker";
};

/**
 * Renders desktop discovery-root consent and recovery controls. Browser and
 * phone selectors keep their existing repository-list actions while discovery
 * diagnostics remain outside the selector UI.
 */
export function RepositoryDiscoveryControls({
  workspaceId,
  enabled = true,
  className,
  presentation = "card",
}: RepositoryDiscoveryControlsProps) {
  const discovery = useRepositoryDiscovery(workspaceId, enabled);
  const { toast } = useToast();
  const { t } = useTranslation();
  const actions = useDiscoveryRootActions(discovery, toast, t);

  if (!enabled || !workspaceId || !discovery.desktopRuntime) return null;

  return (
    <RepositoryDiscoveryRootControls
      className={cn("w-full", className)}
      presentation={presentation}
      isLoading={discovery.isLoading || discovery.isRefreshing}
      discoveryRoots={discovery.rootStates.filter((root) => Boolean(root.id))}
      homeConfirmationRequired={discovery.homeConfirmationRequired}
      onConfirmHomeDiscovery={actions.handleConfirmHomeDiscovery}
      isConfirmingHomeDiscovery={actions.isConfirmingHomeDiscovery}
      onChooseDiscoveryRoot={actions.handleChooseDiscoveryRoot}
      onRefreshDiscovery={actions.refreshDiscovery}
      onReconnectDiscoveryRoot={actions.handleReconnectDiscoveryRoot}
      onRemoveDiscoveryRoot={actions.handleRemoveDiscoveryRoot}
    />
  );
}
