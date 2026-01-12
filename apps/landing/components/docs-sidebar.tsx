'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';

const sections = [
  {
    title: 'Getting Started',
    items: [
      { title: 'Introduction', href: '/docs' },
      { title: 'Installation', href: '/docs/installation' },
      { title: 'Quick Start', href: '/docs/quick-start' },
    ],
  },
  {
    title: 'Core Concepts',
    items: [
      { title: 'Workspaces', href: '/docs/workspaces' },
      { title: 'Boards', href: '/docs/boards' },
      { title: 'Tasks', href: '/docs/tasks' },
    ],
  },
  {
    title: 'Features',
    items: [
      { title: 'Git Worktrees', href: '/docs/worktrees' },
      { title: 'AI Agents', href: '/docs/agents' },
      { title: 'Docker Execution', href: '/docs/docker' },
    ],
  },
  {
    title: 'Integrations',
    items: [
      { title: 'MCP Servers', href: '/docs/mcp' },
      { title: 'GitHub', href: '/docs/github' },
    ],
  },
  {
    title: 'Security',
    items: [
      { title: 'Approval Workflows', href: '/docs/approvals' },
      { title: 'Audit Logs', href: '/docs/audit-logs' },
    ],
  },
  {
    title: 'Advanced',
    items: [
      { title: 'Configuration', href: '/docs/configuration' },
      { title: 'CLI Reference', href: '/docs/cli' },
    ],
  },
];

export function DocsSidebar() {
  const pathname = usePathname();

  return (
    <aside className="sticky top-14 h-[calc(100vh-3.5rem)] w-64 shrink-0 overflow-y-auto border-r border-border py-8 pr-6">
      <nav className="space-y-6">
        {sections.map((section) => (
          <div key={section.title}>
            <h4 className="mb-2 font-semibold text-sm">{section.title}</h4>
            <ul className="space-y-2">
              {section.items.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    className={cn(
                      'block text-sm transition-colors hover:text-foreground',
                      pathname === item.href
                        ? 'font-medium text-foreground'
                        : 'text-muted-foreground'
                    )}
                  >
                    {item.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}
