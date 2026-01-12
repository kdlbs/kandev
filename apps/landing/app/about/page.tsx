import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@kandev/ui/card';
import { IconTarget, IconRocket, IconHeartHandshake } from '@tabler/icons-react';

const values = [
  {
    icon: IconTarget,
    title: 'Developer First',
    description: 'Built by developers, for developers. We understand your workflow because we live it every day.',
  },
  {
    icon: IconRocket,
    title: 'Ship Fast, Ship Safe',
    description: 'Speed without sacrificing security. AI-powered automation with human oversight and approval.',
  },
  {
    icon: IconHeartHandshake,
    title: 'Open & Transparent',
    description: 'Open source at heart. We believe in building in public and learning from the community.',
  },
];

const timeline = [
  {
    year: '2026',
    title: 'The Beginning',
    description: 'kandev.ai is born from the need to combine visual task management with AI-assisted development.',
  },
];

export default function AboutPage() {
  return (
    <div className="container mx-auto py-24 max-w-6xl px-4">
      <div className="flex flex-col gap-4 mb-12">
        <h1 className="text-4xl font-bold">About kandev.ai</h1>
        <p className="text-lg text-muted-foreground">
          We're building the future of development workflows.
        </p>
      </div>

      <div className="prose prose-slate dark:prose-invert max-w-none mb-12">
        <h2>Our Mission</h2>
        <p>
          kandev.ai exists to make software development more visual, more collaborative, and more efficient.
          We believe that combining Kanban methodology with AI-assisted coding creates a workflow that's
          greater than the sum of its parts.
        </p>
        <p>
          Traditional development tools force you to context switch between project management, coding,
          and deployment. We're building a unified experience where your Kanban board is the control center
          for AI agents working in isolated, secure environments.
        </p>
      </div>

      <div className="mb-12">
        <h2 className="text-2xl font-bold mb-6">Our Values</h2>
        <div className="grid gap-6 md:grid-cols-3">
          {values.map((value) => {
            const Icon = value.icon;
            return (
              <Card key={value.title}>
                <CardHeader>
                  <Icon className="h-8 w-8 text-primary mb-2" />
                  <CardTitle>{value.title}</CardTitle>
                </CardHeader>
                <CardContent>
                  <CardDescription>{value.description}</CardDescription>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </div>

      <div className="mb-12">
        <h2 className="text-2xl font-bold mb-6">Our Journey</h2>
        <div className="space-y-6">
          {timeline.map((item) => (
            <Card key={item.year}>
              <CardHeader>
                <div className="text-sm text-muted-foreground mb-1">{item.year}</div>
                <CardTitle>{item.title}</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground">{item.description}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      <div className="prose prose-slate dark:prose-invert max-w-none">
        <h2>Why Kanban + Development + AI?</h2>
        <p>
          Kanban boards provide visual clarity. Development tools provide execution. AI provides automation.
          Together, they create a workflow where you can see all your work, execute it in parallel, and
          automate the routine parts while maintaining full control.
        </p>
        <p>
          We're just getting started. As we grow, we'll continue to focus on making development more
          intuitive, more powerful, and more enjoyable.
        </p>
      </div>
    </div>
  );
}
