import { useCallback, useMemo, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import type { SidebarTaskQuery } from "@/lib/types/http";
import type { SidebarTaskPrefs } from "@/lib/sidebar/apply-view";
import { localSidebarPage } from "@/lib/sidebar/sidebar-local-view";
import { projectLocalSidebarTasks } from "@/lib/sidebar/sidebar-local-projection";

// eslint-disable-next-line max-params -- One shared controller supplies the scoped query, source and preferences.
export function useLocalSidebarPage(
  workspaceId: string | null,
  tasks: TaskOverview[] | null,
  query: SidebarTaskQuery,
  key: string,
  previousPage: number,
  prefs: SidebarTaskPrefs,
) {
  const [selection, setSelection] = useState({ key: "", page: 1 });
  const metadata = useAppStore(
    useShallow((state) => ({
      repositories: state.repositories,
      workflows: state.workflows,
      kanbanMulti: state.kanbanMulti,
    })),
  );
  const page = selection.key === key ? selection.page : previousPage;
  const projected = useMemo(
    () => (tasks && workspaceId ? projectLocalSidebarTasks(tasks, metadata, workspaceId) : null),
    [tasks, metadata, workspaceId],
  );
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  const response = useMemo(
    () =>
      projected
        ? localSidebarPage(
            projected,
            { ...query, page },
            {
              pinnedTaskIds,
              orderedTaskIds,
              subtaskOrderByParentId,
            },
          )
        : null,
    [projected, query, page, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId],
  );
  const lastLocalPage = useRef<{ key: string; page: number } | null>(null);
  if (response) lastLocalPage.current = { key, page: response.page };
  const goToPage = useCallback(
    (requested: number, afterSuccess?: () => void) => {
      if (
        !response ||
        requested < 1 ||
        requested === response.page ||
        requested > response.page + Number(response.has_next)
      )
        return;
      setSelection({ key, page: requested });
      if (afterSuccess) requestAnimationFrame(afterSuccess);
    },
    [response, key],
  );
  return {
    response,
    goToPage,
    previousPage: lastLocalPage.current?.key === key ? lastLocalPage.current.page : 1,
  };
}
