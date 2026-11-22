import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { IconFilter } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

export function TasksListFiltersDrawer({
  count,
  children,
}: {
  count: number;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <Drawer>
      <DrawerTrigger asChild>
        <Button
          variant="outline"
          className="h-11 shrink-0 cursor-pointer gap-2"
          data-testid="tasks-list-filters-trigger"
        >
          <IconFilter className="size-4" aria-hidden="true" />
          {count ? t("tasks:filtersButton", { count }) : t("kanban:filters")}
        </Button>
      </DrawerTrigger>
      <DrawerContent
        data-testid="tasks-list-filters-drawer"
        aria-describedby={undefined}
        className="data-[vaul-drawer-direction=bottom]:max-h-[85dvh]"
      >
        <DrawerHeader className="shrink-0">
          <DrawerTitle>{t("kanban:filters")}</DrawerTitle>
        </DrawerHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-2">
          {children}
        </div>
        <DrawerFooter className="shrink-0 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <DrawerClose asChild>
            <Button className="h-12 cursor-pointer" data-testid="tasks-list-filters-done">
              {t("task:done")}
            </Button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
