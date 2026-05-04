import Link from "next/link";
import { IconBrandGithub, IconBrandSlack, IconHexagon, IconTicket } from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";

const INTEGRATIONS = [
  {
    href: "/settings/integrations/github",
    label: "GitHub",
    description: "PR review queues, issue watchers, and OAuth credentials.",
    Icon: IconBrandGithub,
  },
  {
    href: "/settings/integrations/jira",
    label: "Jira",
    description: "Atlassian Cloud credentials and JQL issue watchers.",
    Icon: IconTicket,
  },
  {
    href: "/settings/integrations/linear",
    label: "Linear",
    description: "Personal API key and team defaults.",
    Icon: IconHexagon,
  },
  {
    href: "/settings/integrations/slack",
    label: "Slack",
    description: "Browser-session credentials and !kandev triage agent.",
    Icon: IconBrandSlack,
  },
];

export default function IntegrationsIndexPage() {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Integrations</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Connect Kandev to third-party services. Credentials are install-wide; per-workspace
          watchers and presets live inside each integration page.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {INTEGRATIONS.map(({ href, label, description, Icon }) => (
          <Link key={href} href={href} className="cursor-pointer">
            <Card className="transition-colors hover:border-primary/40">
              <CardContent className="space-y-2 pt-6">
                <div className="flex items-center gap-2 text-base font-semibold">
                  <Icon className="h-5 w-5" />
                  {label}
                </div>
                <p className="text-sm text-muted-foreground">{description}</p>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
