"use client";

import { IconBrandGithub, IconBrandGitlab } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";
import { cn } from "@/lib/utils";

const PROVIDER_LABELS: Record<RemoteRepositoryProvider, string> = {
  github: "GitHub",
  gitlab: "GitLab",
  azure_devops: "Azure DevOps",
};

export function RemoteRepositoryProviderIcon({ provider }: { provider: RemoteRepositoryProvider }) {
  if (provider === "github") return <IconBrandGithub className="size-3.5 shrink-0" />;
  if (provider === "gitlab") return <IconBrandGitlab className="size-3.5 shrink-0" />;
  return <AzureDevOpsIcon className="size-3.5 shrink-0" />;
}

function ProviderTab({
  provider,
  compact,
}: {
  provider: RemoteRepositoryProvider;
  compact: boolean;
}) {
  const label = PROVIDER_LABELS[provider];
  const trigger = (
    <TabsTrigger
      value={provider}
      aria-label={compact ? label : undefined}
      className={cn(
        "min-h-11 sm:min-h-9 min-w-0 flex-1 cursor-pointer rounded-none after:hidden",
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
        className="min-h-[45px] sm:min-h-[37px] w-full justify-start gap-0 overflow-hidden rounded-none border-t bg-muted/30 p-0"
      >
        {providers.map((provider) => (
          <ProviderTab key={provider} provider={provider} compact={compact} />
        ))}
      </TabsList>
    </Tabs>
  );
}
