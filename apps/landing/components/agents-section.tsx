import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@kandev/ui/card';

const agents = [
  {
    name: 'Claude Code',
    description: "Anthropic's official CLI for Claude. Powerful agent with excellent reasoning and code generation.",
    icon: '🤖',
  },
  {
    name: 'OpenAI Codex',
    description: 'GPT-4 powered coding assistant with strong performance across multiple languages.',
    icon: '🔮',
  },
  {
    name: 'Augment',
    description: 'AI coding assistant focused on code completion and refactoring workflows.',
    icon: '⚡',
  },
  {
    name: 'Opencode',
    description: 'Open-source AI coding agent with customizable models and integrations.',
    icon: '🌐',
  },
];

export function AgentsSection() {
  return (
    <section className="w-full bg-muted/50 py-16 md:py-24">
      <div className="container mx-auto max-w-6xl px-4">
        <div className="flex flex-col items-center gap-4 text-center">
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl md:text-5xl">
            Supported AI Agents
          </h2>
          <p className="max-w-2xl text-lg text-muted-foreground">
            Work with your favorite AI coding assistants. All agents run in isolated, secure environments.
          </p>
        </div>
        <div className="mt-12 grid gap-6 md:grid-cols-2 lg:grid-cols-4">
          {agents.map((agent) => (
            <Card key={agent.name} className="transition-transform hover:scale-105 cursor-pointer">
              <CardHeader>
                <div className="text-4xl mb-2">{agent.icon}</div>
                <CardTitle>{agent.name}</CardTitle>
              </CardHeader>
              <CardContent>
                <CardDescription>{agent.description}</CardDescription>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </section>
  );
}
