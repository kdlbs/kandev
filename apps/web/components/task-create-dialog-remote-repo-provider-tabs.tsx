"use client";

import { IconBrandGithub, IconBrandGitlab, IconGitBranch } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";
import { pluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { cn } from "@/lib/utils";

const PROVIDER_LABELS: Record<string, string> = {
  github: "GitHub",
  gitlab: "GitLab",
  azure_devops: "Azure DevOps",
};

export function RemoteRepositoryProviderIcon({ provider }: { provider: RemoteRepositoryProvider }) {
  if (provider === "github") return <IconBrandGithub className="size-3.5 shrink-0" />;
  if (provider === "gitlab") return <IconBrandGitlab className="size-3.5 shrink-0" />;
  if (provider === "azure_devops") return <AzureDevOpsIcon className="size-3.5 shrink-0" />;
  const registration = pluginRegistry.getRepositoryProvider(provider);
  const Icon = registration ? resolvePluginIcon(registration.icon) : IconGitBranch;
  return <Icon className="size-3.5 shrink-0" />;
}

export function getRemoteRepositoryProviderLabel(provider: RemoteRepositoryProvider): string {
  return (
    PROVIDER_LABELS[provider] ??
    pluginRegistry.getRepositoryProvider(provider)?.label ??
    provider.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
  );
}

function ProviderTab({ provider }: { provider: RemoteRepositoryProvider }) {
  const label = getRemoteRepositoryProviderLabel(provider);
  const trigger = (
    <TabsTrigger
      value={provider}
      className={cn(
        "min-h-11 sm:min-h-9 min-w-max shrink-0 cursor-pointer rounded-none gap-1.5 px-3 after:hidden",
      )}
    >
      <RemoteRepositoryProviderIcon provider={provider} />
      {label}
    </TabsTrigger>
  );

  return trigger;
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
  return (
    <Tabs
      value={value}
      onValueChange={(next) => onChange(next as RemoteRepositoryProvider)}
      className="shrink-0"
    >
      <TabsList
        data-testid="remote-repo-provider-tabs"
        className="min-h-[45px] sm:min-h-[37px] w-full justify-start gap-0 overflow-x-auto rounded-none border-t bg-muted/30 p-0"
      >
        {providers.map((provider) => (
          <ProviderTab key={provider} provider={provider} />
        ))}
      </TabsList>
    </Tabs>
  );
}
