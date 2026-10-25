export type SidebarTaskRepositoryLink = {
  repository_id: string;
  position?: number;
};

/** Resolve a task's repository links into the ordered slugs used by the sidebar. */
export function resolveTaskRepositorySlugs(
  repositoryLinks: readonly SidebarTaskRepositoryLink[] | null | undefined,
  repositorySlugById: ReadonlyMap<string, string | undefined>,
): string[] {
  if (!repositoryLinks || repositoryLinks.length === 0) return [];

  const orderedLinks = repositoryLinks
    .map((link, index) => ({ link, index }))
    .sort((a, b) => {
      const positionA = a.link.position ?? a.index;
      const positionB = b.link.position ?? b.index;
      return positionA - positionB || a.index - b.index;
    });
  const seenRepositoryIds = new Set<string>();
  const slugs: string[] = [];

  for (const { link } of orderedLinks) {
    const repositoryId = String(link.repository_id);
    if (seenRepositoryIds.has(repositoryId)) continue;
    seenRepositoryIds.add(repositoryId);
    const slug = repositorySlugById.get(repositoryId);
    if (slug) slugs.push(slug);
  }

  return slugs;
}
