import { notFound } from 'next/navigation';
import { MDXRemote } from 'next-mdx-remote/rsc';
import { getChangelogEntry, getChangelogSlugs } from '@/lib/changelog';
import { mdxComponents } from '@/components/mdx-components';

function formatDate(date: string) {
  if (!date) return '';
  return new Intl.DateTimeFormat('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  }).format(new Date(date));
}

export async function generateStaticParams() {
  const slugs = await getChangelogSlugs();
  return slugs.map((slug) => ({ slug }));
}

export default async function ChangelogEntryPage({
  params,
}: {
  params: { slug?: string } | Promise<{ slug?: string }>;
}) {
  const resolvedParams = await Promise.resolve(params);
  const slug = resolvedParams.slug;

  if (!slug) {
    notFound();
  }
  const entry = await getChangelogEntry(slug);

  if (!entry) {
    notFound();
  }

  return (
    <div className="relative overflow-hidden">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_top,_rgba(109,40,217,0.12),_transparent_55%)]" />
      <div className="container mx-auto max-w-4xl px-4 pb-24 pt-24">
        <div className="mb-10">
          <p className="text-sm uppercase tracking-[0.3em] text-muted-foreground">{entry.surface}</p>
          <h1 className="mt-4 text-4xl font-semibold text-foreground md:text-5xl">{entry.title}</h1>
          <p className="mt-4 text-sm text-muted-foreground">{formatDate(entry.date)}</p>
        </div>

        <div className="space-y-6">
          {entry.summary.map((line) => (
            <p key={line} className="text-lg text-muted-foreground">
              {line}
            </p>
          ))}
        </div>

        <div className="mt-10 border-t border-border/60 pt-8">
          <MDXRemote source={entry.content} components={mdxComponents} />
        </div>
      </div>
    </div>
  );
}
