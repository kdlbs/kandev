"use client";

import { IconBrandGithub, IconBrandGitlab } from "@tabler/icons-react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import type { RemoteRepositoryProvider } from "@/hooks/domains/integrations/use-remote-repositories";

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
    <Tabs value={value} onValueChange={(next) => onChange(next as RemoteRepositoryProvider)}>
      <TabsList
        data-testid="remote-repo-provider-tabs"
        className="min-h-11 sm:min-h-9 w-full justify-start gap-0 overflow-x-auto rounded-none border-t bg-muted/30 p-0"
      >
        {providers.map((provider) => (
          <TabsTrigger
            key={provider}
            value={provider}
            className="min-h-11 sm:min-h-9 min-w-max flex-1 cursor-pointer gap-1.5 rounded-none px-3"
          >
            <RemoteRepositoryProviderIcon provider={provider} />
            {PROVIDER_LABELS[provider]}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
