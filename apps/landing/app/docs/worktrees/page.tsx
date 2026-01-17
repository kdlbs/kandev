import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

const benefits = [
  { title: 'Parallel tasks', desc: 'Multiple agents can work without branch collisions.' },
  { title: 'Clean diffs', desc: 'Each task produces an isolated patch.' },
  { title: 'Fast setup', desc: 'No extra clones required.' },
];

export default function WorktreesDocsPage() {
  return (
    <DocsShell
      eyebrow="Features"
      title="Git Worktrees"
      description="kandev.ai uses git worktrees to isolate each agent run. Every task gets its own directory that shares the same repository history while keeping file changes separate."
    >
      <section className="grid gap-4 md:grid-cols-3">
        {benefits.map((item) => (
          <DocsCard key={item.title} title={item.title}>
            <p>{item.desc}</p>
          </DocsCard>
        ))}
      </section>

      <DocsCard title="How it looks on disk" variant="muted">
        <DocsCode>{`/repos/kandev
  /main (primary checkout)
  /.worktrees/task-123
  /.worktrees/task-124`}</DocsCode>
      </DocsCard>

      <DocsCard title="Cleanup" variant="accent">
        <p>
          When a task is completed and merged, the worktree can be safely removed without affecting the main checkout.
        </p>
      </DocsCard>
    </DocsShell>
  );
}
