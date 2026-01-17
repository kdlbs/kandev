import Link from 'next/link';
import { getChangelogEntries } from '@/lib/changelog';

function formatDate(date: string) {
  if (!date) return '';
  return new Intl.DateTimeFormat('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  }).format(new Date(date));
}

export default async function ChangelogPage() {
  const entries = await getChangelogEntries();

  return (
    <div className="relative overflow-hidden">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_top,_rgba(109,40,217,0.12),_transparent_55%)]" />
      <div className="container mx-auto max-w-6xl px-4 pb-24 pt-24">
        <div className="mb-12">
          <p className="text-sm uppercase tracking-[0.3em] text-muted-foreground">Changelog</p>
          <h1 className="mt-4 text-4xl font-semibold text-foreground md:text-5xl">Product updates</h1>
          <p className="mt-4 max-w-2xl text-lg text-muted-foreground">
            A running log of kandev.ai releases, shipped features, and operational improvements.
          </p>
        </div>

        <div className="space-y-12">
          {entries.map((entry, index) => (
            <article
              key={entry.slug}
              className="grid gap-6 border-b border-border/60 pb-12 md:grid-cols-[180px_1fr]"
            >
              <div className="text-sm text-muted-foreground">
                <p className="font-medium text-foreground">{entry.surface}</p>
                <p>{formatDate(entry.date)}</p>
                {index === 0 ? (
                  <span className="mt-3 inline-flex items-center rounded-full border border-border bg-background px-2 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] text-foreground">
                    Latest
                  </span>
                ) : null}
              </div>

              <div>
                <h2 className="text-2xl font-semibold text-foreground">
                  <Link href={`/changelog/${entry.slug}`} className="hover:text-primary">
                    {entry.title}
                  </Link>
                </h2>
                <div className="mt-4 space-y-3 text-muted-foreground">
                  {entry.summary.map((line) => (
                    <p key={line}>{line}</p>
                  ))}
                </div>
                <div className="mt-6">
                  <Link
                    href={`/changelog/${entry.slug}`}
                    className="text-sm font-semibold text-primary hover:text-primary/80"
                  >
                    Read full update →
                  </Link>
                </div>
              </div>
            </article>
          ))}
        </div>
      </div>
    </div>
  );
}
