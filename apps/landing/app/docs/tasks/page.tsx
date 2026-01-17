import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function TasksDocsPage() {
  return (
    <DocsShell
      eyebrow="Core Concepts"
      title="Tasks"
      description="Tasks capture the work that agents will execute. A good task includes a goal, constraints, and the desired output so the agent can plan accurately."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="Recommended fields">
          <ul className="space-y-2">
            <li>Title and description</li>
            <li>Priority and due date</li>
            <li>Repository and base branch</li>
            <li>Agent type and environment</li>
            <li>Approval requirements</li>
          </ul>
        </DocsCard>
        <DocsCard title="Writing effective tasks" variant="accent">
          <ol className="space-y-2 list-decimal list-inside">
            <li>State the outcome, not the steps.</li>
            <li>Call out sensitive files or commands.</li>
            <li>Include acceptance criteria.</li>
          </ol>
        </DocsCard>
      </section>

      <DocsCard title="Example task" variant="muted">
        <DocsCode>{`Title: Add changelog page
Repo: github.com/kdlbs/kandev
Branch: main
Agent: augment-agent
Goal: Replace blog with changelog
Constraints: Keep existing navbar/footer
Output: New route + MDX entries`}</DocsCode>
      </DocsCard>
    </DocsShell>
  );
}
