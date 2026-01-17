import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

const steps = [
  {
    step: '1. Create a workspace',
    body: 'Create a workspace for the repository you want to manage.',
  },
  {
    step: '2. Add a board and task',
    body: 'Add a board with columns, then create a task that describes the desired change.',
  },
  {
    step: '3. Run an agent',
    body: 'Start an agent from the task. The UI should stream progress updates as the agent works.',
  },
  {
    step: '4. Review and merge',
    body: 'Review the diff, approve any gated actions, and merge the worktree.',
  },
];

export default function QuickStartDocsPage() {
  return (
    <DocsShell
      eyebrow="Getting Started"
      title="Quick Start"
      description="This guide walks through a minimal workflow so you can see the agent loop end-to-end."
    >
      <section className="grid gap-4 md:grid-cols-2">
        {steps.map((item) => (
          <DocsCard key={item.step} title={item.step}>
            <p>{item.body}</p>
          </DocsCard>
        ))}
      </section>

      <DocsCard title="Example task prompt" variant="muted">
        <DocsCode>{`Task: Update onboarding copy
Constraints: Touch only apps/landing
Acceptance: Copy updated and passes lint`}</DocsCode>
      </DocsCard>
    </DocsShell>
  );
}
