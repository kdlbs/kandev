import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

const stages = [
  { title: 'Backlog', desc: 'Collect ideas and future work.' },
  { title: 'In Progress', desc: 'Active tasks owned by agents.' },
  { title: 'Review', desc: 'Approvals and QA before merge.' },
  { title: 'Done', desc: 'Completed and shipped items.' },
];

export default function BoardsDocsPage() {
  return (
    <DocsShell
      eyebrow="Core Concepts"
      title="Boards"
      description="Boards are the Kanban surface for your workspace. Each board defines columns that represent the lifecycle of a task, from backlog to review to done."
    >
      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {stages.map((stage) => (
          <DocsCard key={stage.title} title={stage.title}>
            <p>{stage.desc}</p>
          </DocsCard>
        ))}
      </section>

      <DocsCard title="Example board config" variant="muted">
        <p>Keep columns minimal and apply WIP limits where review capacity is tight.</p>
        <DocsCode>{`board:
  name: "Product Roadmap"
  columns:
    - name: "Backlog"
      wipLimit: null
    - name: "In Progress"
      wipLimit: 4
    - name: "Review"
      wipLimit: 2
    - name: "Done"
      wipLimit: null`}</DocsCode>
      </DocsCard>

      <DocsCard title="Tips" variant="accent">
        <ul className="space-y-2">
          <li>Use a single “Review” column so approvals are centralized.</li>
          <li>Keep WIP limits low to prevent agent overload.</li>
          <li>Archive finished tasks weekly to keep focus on active work.</li>
        </ul>
      </DocsCard>
    </DocsShell>
  );
}
