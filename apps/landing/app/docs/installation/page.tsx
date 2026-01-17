import { DocsShell } from '@/components/docs-shell';
import { DocsCard } from '@/components/docs-card';
import { DocsCode } from '@/components/docs-code';

export default function InstallationDocsPage() {
  return (
    <DocsShell
      eyebrow="Getting Started"
      title="Installation"
      description="These steps outline a typical local installation. Exact commands may change as we finalize the installer, but the flow remains the same."
    >
      <section className="grid gap-4 md:grid-cols-2">
        <DocsCard title="Prerequisites" variant="accent">
          <ul className="space-y-2">
            <li>Docker Desktop (or Docker Engine)</li>
            <li>Git 2.30+</li>
            <li>Node.js 20+</li>
          </ul>
        </DocsCard>
        <DocsCard title="Install the repo">
          <DocsCode>{`git clone https://github.com/kdlbs/kandev.git
cd kandev
pnpm install`}</DocsCode>
        </DocsCard>
      </section>

      <DocsCard title="Run the services" variant="muted">
        <p>Start the backend, app, and landing site in separate terminals.</p>
        <DocsCode>{`pnpm --filter @kandev/backend dev
pnpm --filter @kandev/web dev
pnpm --filter @kandev/landing dev`}</DocsCode>
      </DocsCard>

      <DocsCard title="Verify">
        <p>
          Open the UI and create a test workspace. The system should start an agent container on demand and stream
          progress back over WebSocket.
        </p>
      </DocsCard>
    </DocsShell>
  );
}
