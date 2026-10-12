import type { SidebarLayout } from "./layout-types";
export type SidebarMutation = (layout: SidebarLayout) => SidebarLayout;
export function createSidebarWriter({
  read,
  write,
}: {
  read: (workspaceId: string) => SidebarLayout;
  write: (workspaceId: string, layout: SidebarLayout) => Promise<void>;
}) {
  let tail: Promise<void> = Promise.resolve();
  return (workspaceId: string, mutation: SidebarMutation): Promise<void> => {
    const job = tail
      .catch(() => undefined)
      .then(() => write(workspaceId, mutation(read(workspaceId))));
    tail = job;
    return job;
  };
}
