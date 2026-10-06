"use client";

import { IconBrandGithub, IconBrandGitlab, IconGitBranch } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { cn } from "@/lib/utils";

const PROVIDER_LABELS: Record<string, string> = {
  github: "GitHub",
  gitlab: "GitLab",
  azure_devops: "Azure DevOps",
};

export function RemoteRepositoryProviderIcon({ provider }: { provider: RemoteRepositoryProvider }) {
  const registry = usePluginRegistry();
  if (provider === "github") return <IconBrandGithub className="size-3.5 shrink-0" />;
  if (provider === "gitlab") return <IconBrandGitlab className="size-3.5 shrink-0" />;
  if (provider === "azure_devops") return <AzureDevOpsIcon className="size-3.5 shrink-0" />;
  const registration = registry.getRepositoryProvider(provider);
  const Icon = registration ? resolvePluginIcon(registration.icon) : IconGitBranch;
  return <Icon className="size-3.5 shrink-0" />;
}

export function remoteRepositoryProviderLabel(
  provider: RemoteRepositoryProvider,
  registeredLabel?: string,
): string {
  return (
    PROVIDER_LABELS[provider] ??
    registeredLabel ??
    provider.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
  );
}

export function useRemoteRepositoryProviderLabel(provider: RemoteRepositoryProvider): string {
  const registry = usePluginRegistry();
  return remoteRepositoryProviderLabel(provider, registry.getRepositoryProvider(provider)?.label);
}

function ProviderTab({
  provider,
  compact,
}: {
  provider: RemoteRepositoryProvider;
  compact: boolean;
}) {
  const label = useRemoteRepositoryProviderLabel(provider);
  const trigger = (
    <TabsTrigger
      value={provider}
      aria-label={compact ? label : undefined}
      className={cn(
        controlSizingClassName(
          "standard",
          "min-w-0 flex-1 cursor-pointer rounded-none after:hidden",
        ),
        compact ? "px-2" : "gap-1.5 px-3",
      )}
    >
      <RemoteRepositoryProviderIcon provider={provider} />
      {compact ? null : label}
    </TabsTrigger>
  );

  if (!compact) return trigger;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent side="top">{label}</TooltipContent>
    </Tooltip>
  );
}

export function RemoteRepoProviderTabs({
  providers,
  value,
  onChange,
}: {
  providers: RemoteRepositoryProvider[];
  value: RemoteRepositoryProvider;
  onChange: (provider: RemoteRepositoryProvider) => void;
}) {
  const compact = providers.length >= 3;
  return (
    <Tabs
      value={value}
      onValueChange={(next) => onChange(next as RemoteRepositoryProvider)}
      className="shrink-0"
    >
      <TabsList
        data-testid="remote-repo-provider-tabs"
        className={cn(
          controlSizingClassName(
            "standard",
            "relative w-full justify-start gap-0 overflow-hidden rounded-none bg-muted/30 p-0 before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-border",
          ),
          "rounded-none",
        )}
      >
        {providers.map((provider) => (
          <ProviderTab key={provider} provider={provider} compact={compact} />
        ))}
      </TabsList>
    </Tabs>
  );
}
