import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { Input } from "@kandev/ui/input";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import type { CoordinatorFilters } from "@/lib/orchestration/coordinator-task-observation";
import { CoordinatorSelect } from "./coordinator-select";

export function TaskFilters({
  catalog,
  filters,
  setFilters,
  scope,
  setScope,
  selected,
}: {
  catalog: CoordinatorWorkspace;
  filters: CoordinatorFilters;
  setFilters: (next: CoordinatorFilters) => void;
  scope: string;
  setScope: (value: string) => void;
  selected: string;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  return (
    <div className="grid grid-cols-2 gap-3 p-4">
      <div className="col-span-2">
        <label className="text-xs text-muted-foreground" htmlFor="coordinator-search">
          {t("orchestration:searchTasks")}
        </label>
        <Input
          id="coordinator-search"
          value={filters.query ?? ""}
          onChange={(e) => setFilters({ ...filters, query: e.target.value })}
          className="mt-1 max-md:min-h-11"
        />
      </div>
      <details open={!isMobile} className="col-span-2">
        <summary className="md:hidden cursor-pointer min-h-11 flex items-center text-sm">
          {t("task:filters")}
        </summary>
        <div className="grid grid-cols-2 gap-3">
          <CoordinatorSelect
            label={t("orchestration:workflowFilter")}
            value={filters.workflowId || "all"}
            onChange={(id) => setFilters({ ...filters, workflowId: id === "all" ? null : id })}
            options={[
              { id: "all", name: t("orchestration:allWorkflows") },
              ...catalog.workflows.filter(
                (flow) => flow.id !== catalog.workspace.office_workflow_id,
              ),
            ]}
          />
          <CoordinatorSelect
            label={t("orchestration:repositoryFilter")}
            value={filters.repositoryId || "all"}
            onChange={(id) => setFilters({ ...filters, repositoryId: id === "all" ? null : id })}
            options={[
              { id: "all", name: t("orchestration:allRepositories") },
              ...catalog.repositories,
            ]}
          />
          <div className="col-span-2">
            <CoordinatorSelect
              label={t("orchestration:taskScope")}
              value={scope}
              onChange={setScope}
              options={[
                { id: "all", name: t("orchestration:allTasks") },
                ...(selected
                  ? [{ id: "coordinated", name: t("orchestration:selectedTasks") }]
                  : []),
              ]}
            />
          </div>
        </div>
      </details>
    </div>
  );
}
