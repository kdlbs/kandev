'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { IconBrandGithub } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { cn } from '@/lib/utils';

export function Navbar() {
  const pathname = usePathname();
  const stars = '2k'; // Dummy star count display

  return (
    <header className="fixed top-0 z-50 w-full border-b border-border/40 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
        <Link href="/" className="flex items-center space-x-2">
          <span className="font-bold">kandev.ai</span>
        </Link>

        <nav className="flex items-center gap-6 text-sm">
          <Link
            href="/#product"
            className={cn(
              'text-sm font-medium transition-colors hover:text-foreground cursor-pointer',
              pathname === '/' ? 'text-foreground' : 'text-muted-foreground'
            )}
          >
            Product
          </Link>
          <Link
            href="/docs"
            className={cn(
              'text-sm font-medium transition-colors hover:text-foreground cursor-pointer',
              pathname?.startsWith('/docs') ? 'text-foreground' : 'text-muted-foreground'
            )}
          >
            Documentation
          </Link>
          <Link
            href="/changelog"
            className={cn(
              'text-sm font-medium transition-colors hover:text-foreground cursor-pointer',
              pathname?.startsWith('/changelog') ? 'text-foreground' : 'text-muted-foreground'
            )}
          >
            Changelog
          </Link>
        </nav>

        <div className="flex items-center space-x-2">
          <Button variant="outline" size="sm" asChild>
            <a
              href="https://github.com/kdlbs/kandev"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2"
            >
              <IconBrandGithub className="h-4 w-4" />
              <span>Star</span>
              <span className="ml-1 rounded-full bg-muted px-2 py-0.5 text-xs font-medium">
                {stars}
              </span>
            </a>
          </Button>
        </div>
      </div>
    </header>
  );
}
