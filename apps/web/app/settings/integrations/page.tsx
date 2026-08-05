import Link from "@/components/routing/app-link";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandAzure,
  IconBrandSentry,
  IconBrandSlack,
  IconHexagon,
  IconTicket,
} from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";
import { useTranslation } from "react-i18next";

const INTEGRATIONS = [
  {
    slug: "azure-devops",
    label: "Azure DevOps",
    descriptionKey: "settings:integrationDescriptionAzureDevops",
    Icon: IconBrandAzure,
  },
  {
    slug: "github",
    label: "GitHub",
    descriptionKey: "settings:integrationDescriptionGithub",
    Icon: IconBrandGithub,
  },
  {
    slug: "gitlab",
    label: "GitLab",
    descriptionKey: "settings:integrationDescriptionGitlab",
    Icon: IconBrandGitlab,
  },
  {
    slug: "jira",
    label: "Jira",
    descriptionKey: "settings:integrationDescriptionJira",
    Icon: IconTicket,
  },
  {
    slug: "linear",
    label: "Linear",
    descriptionKey: "settings:integrationDescriptionLinear",
    Icon: IconHexagon,
  },
  {
    slug: "sentry",
    label: "Sentry",
    descriptionKey: "settings:integrationDescriptionSentry",
    Icon: IconBrandSentry,
  },
  {
    slug: "slack",
    label: "Slack",
    descriptionKey: "settings:integrationDescriptionSlack",
    Icon: IconBrandSlack,
  },
];

type IntegrationsIndexPageProps = {
  workspaceId?: string;
};

export default function IntegrationsIndexPage({ workspaceId }: IntegrationsIndexPageProps = {}) {
  const { t } = useTranslation();
  const rootHref = workspaceId
    ? `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations`
    : "/settings/integrations";

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("common:integrations")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("settings:connectKandevToThirdPartyServices")}
        </p>
      </div>
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {INTEGRATIONS.map(({ slug, label, descriptionKey, Icon }) => {
          const href = `${rootHref}/${slug}`;
          return (
            <Link key={href} href={href} className="flex h-full cursor-pointer">
              <Card className="h-full w-full transition-colors hover:border-primary/40">
                <CardContent className="space-y-2">
                  <div className="flex items-center gap-2 text-base font-semibold">
                    <Icon className="h-5 w-5" />
                    {label}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {t(descriptionKey, { trigger: "!kandev" })}
                  </p>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
