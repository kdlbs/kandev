import type { ComponentProps, RefObject } from "react";
import type { useSidebarTaskPage } from "@/hooks/domains/kanban/use-sidebar-task-page";
import { TaskSwitcher } from "./task-switcher";
import { SidebarTaskQueryStatus } from "./sidebar-task-query-status";
import { SidebarTaskPagination } from "./sidebar-task-pagination";

export function SidebarTaskPageContent({
  page,
  workspaceContextError,
  switcherProps,
  scrollRef,
}: {
  page: ReturnType<typeof useSidebarTaskPage>;
  workspaceContextError: string | null;
  switcherProps: ComponentProps<typeof TaskSwitcher>;
  scrollRef: RefObject<HTMLDivElement | null>;
}) {
  return (
    <>
      {!workspaceContextError && (
        <SidebarTaskQueryStatus
          error={page.error}
          pending={page.requestedPage !== null}
          hasPage={page.response !== null}
          canRetry={page.canRetry}
          onRetry={page.retry}
        />
      )}
      {!(page.error && !page.response && !workspaceContextError) && (
        <TaskSwitcher {...switcherProps} />
      )}
      <SidebarTaskPagination
        page={page.response}
        pending={page.requestedPage !== null || page.isDisclosureTransition}
        onPageChange={(nextPage) =>
          page.goToPage(nextPage, () => scrollRef.current?.scrollTo({ top: 0 }))
        }
      />
    </>
  );
}
