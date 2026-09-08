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
 * The shared consent and recovery surface for every repository selector.
 * Keeping the discovery lease and root actions together prevents a picker
 * from showing an empty result without also offering the action that can
 * establish or repair its filesystem access.
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
      onChooseDiscoveryRoot={actions.handleChooseDiscoveryRoot}
      onRefreshDiscovery={actions.refreshDiscovery}
      onReconnectDiscoveryRoot={actions.handleReconnectDiscoveryRoot}
      onRemoveDiscoveryRoot={actions.handleRemoveDiscoveryRoot}
    />
  );
}
