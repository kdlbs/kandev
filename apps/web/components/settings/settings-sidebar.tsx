'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { IconSettings, IconFolder, IconServer, IconRobot } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

const NAV_ITEMS = [
  {
    href: '/settings/general',
    label: 'General',
    icon: IconSettings,
  },
  {
    href: '/settings/workspace',
    label: 'Workspace',
    icon: IconFolder,
  },
  {
    href: '/settings/environments',
    label: 'Environments',
    icon: IconServer,
  },
  {
    href: '/settings/agents',
    label: 'Agents',
    icon: IconRobot,
  },
];

type SettingsSidebarProps = {
  className?: string;
  onNavigate?: () => void;
};

export function SettingsSidebar({ className, onNavigate }: SettingsSidebarProps) {
  const pathname = usePathname();

  return (
    <nav className={cn('w-64 border-r border-border bg-card flex-col p-4', className)}>
      <div className="space-y-1">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon;
          const isActive = pathname === item.href;

          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={onNavigate}
              className={cn(
                'flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground font-medium'
                  : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
              )}
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
