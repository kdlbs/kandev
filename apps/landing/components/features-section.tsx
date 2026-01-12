import { IconGitBranch, IconCode, IconShieldCheck, IconPlugConnected, IconFolders, IconBrandDocker } from '@tabler/icons-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@kandev/ui/card';

const features = [
  {
    icon: IconCode,
    title: 'Parallel Agent Execution',
    description:
      'Run multiple AI coding agents simultaneously on different tasks. Each agent works in isolated environments, accelerating development without conflicts.',
  },
  {
    icon: IconGitBranch,
    title: 'Isolated Worktrees',
    description:
      'Each task gets its own git worktree. Work on multiple features, bugs, and experiments in parallel without branch switching or stashing.',
  },
  {
    icon: IconShieldCheck,
    title: 'Built-in Code Review',
    description:
      'Automated code review and approval workflows. Review agent changes before merging, with full audit logs and rollback capabilities.',
  },
  {
    icon: IconPlugConnected,
    title: 'MCP Integration',
    description:
      'Connect with Model Context Protocol servers. Extend agent capabilities with custom tools, APIs, and integrations.',
  },
  {
    icon: IconFolders,
    title: 'Multiple Workspaces',
    description:
      'Organize projects by client, team, or product. Switch between workspaces seamlessly and keep your work isolated.',
  },
  {
    icon: IconBrandDocker,
    title: 'Local Docker',
    description:
      'AI agents run in local Docker containers for complete isolation. Secure execution with approval workflows and audit logs for every operation.',
  },
];

export function FeaturesSection() {
  return (
    <section id="features" className="container mx-auto max-w-6xl py-16 md:py-24 px-4">
      <div className="flex flex-col items-center gap-4 text-center">
        <h2 className="text-3xl font-bold tracking-tight sm:text-4xl md:text-5xl">
          Build for development velocity
        </h2>
        <p className="max-w-2xl text-lg text-muted-foreground">
          Combine the visual clarity of Kanban boards with the power of AI-assisted development.
        </p>
      </div>
      <div className="mt-12 grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        {features.map((feature) => {
          const Icon = feature.icon;
          return (
            <Card key={feature.title}>
              <CardHeader>
                <Icon className="h-8 w-8 text-primary mb-2" />
                <CardTitle>{feature.title}</CardTitle>
              </CardHeader>
              <CardContent>
                <CardDescription>{feature.description}</CardDescription>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </section>
  );
}
