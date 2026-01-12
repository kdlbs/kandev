import Link from 'next/link';
import { IconBrandGithub, IconBrandTwitter, IconBrandDiscord } from '@tabler/icons-react';

export function Footer() {
  return (
    <footer className="border-t border-border bg-background">
      <div className="container mx-auto max-w-6xl py-12 md:py-16 px-4">
        <div className="grid gap-8 md:grid-cols-3">
          <div>
            <h3 className="font-bold mb-4">kandev.ai</h3>
            <p className="text-sm text-muted-foreground">
              AI-powered Kanban for developers. Ship faster with isolated worktrees and secure Docker execution.
            </p>
          </div>
          <div>
            <h4 className="font-semibold mb-4">Product</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/#features" className="text-muted-foreground hover:text-foreground">
                  Features
                </Link>
              </li>
              <li>
                <Link href="/docs" className="text-muted-foreground hover:text-foreground">
                  Documentation
                </Link>
              </li>
            </ul>
          </div>
          <div>
            <h4 className="font-semibold mb-4">Company</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/about" className="text-muted-foreground hover:text-foreground">
                  About
                </Link>
              </li>
              <li>
                <Link href="/blog" className="text-muted-foreground hover:text-foreground">
                  Blog
                </Link>
              </li>
            </ul>
          </div>
        </div>
        <div className="mt-8 border-t border-border pt-8 flex flex-col md:flex-row justify-between items-center gap-4">
          <p className="text-sm text-muted-foreground">
            © 2026 kandev.ai. All rights reserved. Built with respect for your privacy and GDPR compliance.
          </p>
          <div className="flex gap-4">
            <a
              href="https://github.com/kdlbs/kandev"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground"
            >
              <IconBrandGithub className="h-5 w-5" />
            </a>
            <a
              href="https://twitter.com/kandev_ai"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground"
            >
              <IconBrandTwitter className="h-5 w-5" />
            </a>
            <a
              href="https://discord.gg/kandev"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground"
            >
              <IconBrandDiscord className="h-5 w-5" />
            </a>
          </div>
        </div>
      </div>
    </footer>
  );
}
