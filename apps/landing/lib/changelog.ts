import fs from 'node:fs/promises';
import path from 'node:path';
import matter from 'gray-matter';

export type ChangelogEntry = {
  slug: string;
  title: string;
  date: string;
  surface: string;
  summary: string[];
  content: string;
};

const changelogRoot = path.join(process.cwd(), 'content', 'changelog');

function normalizeSummary(summary: unknown): string[] {
  if (Array.isArray(summary)) {
    return summary.filter((item): item is string => typeof item === 'string');
  }
  if (typeof summary === 'string') {
    return [summary];
  }
  return [];
}

function parseEntry(slug: string, fileContent: string): ChangelogEntry {
  const { data, content } = matter(fileContent);

  return {
    slug,
    title: String(data.title ?? slug),
    date: String(data.date ?? ''),
    surface: String(data.surface ?? 'Product'),
    summary: normalizeSummary(data.summary),
    content,
  };
}

export async function getChangelogEntries(): Promise<ChangelogEntry[]> {
  const entries = await fs.readdir(changelogRoot, { withFileTypes: true });
  const slugs = entries.filter((entry) => entry.isDirectory()).map((entry) => entry.name);

  const items = await Promise.all(
    slugs.map(async (slug) => {
      const entryPath = path.join(changelogRoot, slug, 'index.mdx');
      const fileContent = await fs.readFile(entryPath, 'utf8');
      return parseEntry(slug, fileContent);
    })
  );

  return items.sort((a, b) => {
    const aTime = new Date(a.date).getTime();
    const bTime = new Date(b.date).getTime();
    return bTime - aTime;
  });
}

export async function getChangelogEntry(slug: string): Promise<ChangelogEntry | null> {
  const entryPath = path.join(changelogRoot, slug, 'index.mdx');
  try {
    const fileContent = await fs.readFile(entryPath, 'utf8');
    return parseEntry(slug, fileContent);
  } catch {
    return null;
  }
}

export async function getChangelogSlugs(): Promise<string[]> {
  const entries = await fs.readdir(changelogRoot, { withFileTypes: true });
  return entries.filter((entry) => entry.isDirectory()).map((entry) => entry.name);
}
