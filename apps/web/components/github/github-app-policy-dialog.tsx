"use client";

import { IconShieldCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import { useTranslation } from "react-i18next";

export const githubAppPermissions: Record<string, "read" | "write"> = {
  actions: "read",
  administration: "read",
  checks: "read",
  contents: "write",
  issues: "write",
  members: "read",
  metadata: "read",
  pull_requests: "write",
  statuses: "read",
  workflows: "write",
};

export const githubAppEvents = [
  "installation",
  "installation_repositories",
  "github_app_authorization",
];

function title(value: string) {
  return value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

export function GitHubAppPolicyDialog({
  permissions = githubAppPermissions,
  events = githubAppEvents,
}: {
  permissions?: Record<string, "read" | "write">;
  events?: string[];
}) {
  const { t } = useTranslation();
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" className="h-11 cursor-pointer">
          <IconShieldCheck className="mr-2 h-4 w-4" />
          {t("github:reviewPermissions")}
        </Button>
      </DialogTrigger>
      <DialogContent className="flex max-h-[80dvh] flex-col overflow-hidden sm:max-w-lg">
        <DialogHeader className="shrink-0">
          <DialogTitle>{t("github:requiredGithubAppPolicy")}</DialogTitle>
          <DialogDescription>{t("github:anImportedAppMustHaveAt")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain">
          <section>
            <h3 className="mb-2 text-sm font-medium">{t("github:repositoryPermissions")}</h3>
            <div className="divide-y rounded-md border">
              {Object.entries(permissions)
                .sort(([left], [right]) => left.localeCompare(right))
                .map(([name, level]) => (
                  <div
                    key={name}
                    className="flex min-h-11 items-center justify-between gap-3 px-3 text-sm"
                  >
                    <span className="min-w-0 break-words">{title(name)}</span>
                    <span className="shrink-0 capitalize text-muted-foreground">{level}</span>
                  </div>
                ))}
            </div>
          </section>
          <section>
            <h3 className="mb-2 text-sm font-medium">{t("github:subscribedEvents")}</h3>
            <div className="divide-y rounded-md border">
              {events.map((event) => (
                <div key={event} className="flex min-h-11 items-center px-3 text-sm">
                  {title(event)}
                </div>
              ))}
            </div>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
