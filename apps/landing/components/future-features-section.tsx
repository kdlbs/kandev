import { IconCloud, IconDeviceMobile, IconUsers } from '@tabler/icons-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@kandev/ui/card';
import { Badge } from '@kandev/ui/badge';

const futureFeatures = [
  {
    icon: IconCloud,
    title: 'Remote Executors',
    description: 'Run AI agents on cloud or remote machines. Scale your development with powerful remote compute.',
    status: 'Coming Soon',
  },
  {
    icon: IconDeviceMobile,
    title: 'Mobile App',
    description: 'Launch and monitor tasks from your mobile device. Stay connected to your development workflow.',
    status: 'In Development',
  },
  {
    icon: IconUsers,
    title: 'Team Collaboration',
    description: 'Share workspaces, assign tasks, and collaborate with your team in real-time.',
    status: 'Planned',
  },
];

export function FutureFeaturesSection() {
  return (
    <section className="container mx-auto max-w-6xl py-16 md:py-24 px-4">
      <div className="flex flex-col items-center gap-4 text-center">
        <h2 className="text-3xl font-bold tracking-tight sm:text-4xl md:text-5xl">
          Coming Soon
        </h2>
        <p className="max-w-2xl text-lg text-muted-foreground">
          We're constantly improving kandev.ai. Here's what's on our roadmap.
        </p>
      </div>
      <div className="mt-12 grid gap-6 md:grid-cols-3">
        {futureFeatures.map((feature) => {
          const Icon = feature.icon;
          return (
            <Card key={feature.title} className="relative">
              <CardHeader>
                <Icon className="h-8 w-8 text-primary mb-2" />
                <div className="flex items-center justify-between">
                  <CardTitle>{feature.title}</CardTitle>
                </div>
                <Badge variant="secondary" className="w-fit">
                  {feature.status}
                </Badge>
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
