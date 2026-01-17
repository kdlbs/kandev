import { DocsSidebar } from '@/components/docs-sidebar';

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(109,40,217,0.12),_transparent_55%)]">
      <div className="mx-auto flex w-full max-w-6xl gap-8 px-6">
        <DocsSidebar />
        <main className="flex-1 py-16">{children}</main>
      </div>
    </div>
  );
}
