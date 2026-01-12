import { DocsSidebar } from '@/components/docs-sidebar';

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen pt-16 justify-center">
      <DocsSidebar />
      <main className="flex-1 max-w-4xl py-8 px-6">{children}</main>
    </div>
  );
}
