import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function WorkspacesDocsPage() {
  return (
    <DocsShell
      eyebrow="Core Concepts"
      title="Workspaces"
      description="Workspaces are the top-level container in kandev.ai. They group repositories, boards, agent settings, and members so teams can ship without stepping on each other."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="What a workspace includes">
          <ul className="space-y-2">
            <li>Boards that define workflow stages</li>
            <li>Repositories connected to those boards</li>
            <li>Default agent settings and approvals</li>
            <li>Member roles and permissions</li>
          </ul>
        </DocsCard>
        <DocsCard title="Best practices" variant="accent">
          <ol className="space-y-2 list-decimal list-inside">
            <li>Use one workspace per product or client.</li>
            <li>Keep board naming consistent across teams.</li>
            <li>Set approvals at the workspace level.</li>
          </ol>
        </DocsCard>
      </section>

      <DocsCard title="Example structure" variant="muted">
        <p>A workspace keeps everything organized in one place.</p>
        <DocsCode>{`Workspace: "Core Platform"
Boards:
  - Product Roadmap
  - Incident Response
Repositories:
  - github.com/kdlbs/kandev
  - github.com/kdlbs/kandev-docs`}</DocsCode>
      </DocsCard>
    </DocsShell>
  );
}
