"use client";
import { MobileListingOptionsContext } from "./mobile-listing-options-context";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";
import { type ReactNode, type RefObject } from "react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { IconColumns, IconLayoutKanban, IconList, IconTimeline } from "@tabler/icons-react";
import { MobileWorkspaceActionsSection } from "@/components/app-sidebar/app-sidebar-workspace-actions";
import { AppSidebarWorkspacePicker } from "@/components/app-sidebar/app-sidebar-workspace-picker";
import {
  AppNavSections,
  useAppNavDialogs,
  type AppNavDialogControls,
} from "@/components/navigation/app-nav-sections";
import { TaskSearchInput } from "./task-search-input";
import type { TasksListDisplayOptions } from "./mobile-menu-task-list-options";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useMobileMenuSheetState } from "@/hooks/use-mobile-menu-sheet-state";
import { MobileDisplayOptions } from "./mobile-display-options";
import type { MobileDisplayOptionsProps } from "./mobile-display-options";
export type { MobileDisplayOptionsProps, MobileColumnsSection } from "./mobile-display-options";
import {
  mobileControlClass,
  mobileControlIconClass,
  mobileSectionClass,
  mobileSectionTitleClass,
} from "./mobile-menu-styles";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";
export type MobileMenuSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus?: (event: Event) => void;
  workspaceId?: string;
  currentPage?: TaskListingPage;
  searchQuery?: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading?: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  pageActions?: ReactNode;
  listingOnly?: boolean;
  listingControls?: ReactNode;
};

function MobileSearchSection({
  searchQuery,
  onSearchChange,
  isSearchLoading,
}: {
  searchQuery: string;
  onSearchChange?: (query: string) => void;
  isSearchLoading: boolean;
}) {
  const { t } = useTranslation();
  if (!onSearchChange) return null;

  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("kanban:searchSection")}</label>
      <TaskSearchInput
        value={searchQuery}
        onChange={onSearchChange}
        placeholder={t("kanban:searchTasksPlaceholder")}
        isLoading={isSearchLoading}
        className="w-full [&_[data-slot=input]]:h-10 [&_[data-slot=input]]:pl-9 [&_[data-slot=input]]:pr-9 [&_[data-slot=input]]:text-sm"
      />
    </div>
  );
}

function MobileWorkspaceSection({ onOpenChange }: { onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("common:workspace")}</label>
      <AppSidebarWorkspacePicker
        modal={false}
        onActionComplete={() => onOpenChange(false)}
        triggerClassName={cn("flex-none", mobileControlClass)}
        triggerTestId="mobile-workspace-trigger"
        chevronTestId="mobile-workspace-trigger-chevron"
        itemTestIdPrefix="mobile-workspace-item"
        contentClassName="w-80 max-w-[calc(100vw-2rem)]"
      />
    </div>
  );
}

function MobileViewSection({
  viewValue,
  onViewChange,
  showPipeline,
}: {
  viewValue: string;
  onViewChange: (value: string) => void;
  showPipeline: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className={mobileSectionClass}>
      <label className={mobileSectionTitleClass}>{t("kanban:view")}</label>
      <ToggleGroup
        type="single"
        value={viewValue}
        onValueChange={onViewChange}
        variant="outline"
        className="w-full justify-start"
      >
        <ToggleGroupItem
          value="kanban"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconLayoutKanban className={mobileControlIconClass} />
          {t("kanban:kanban")}
        </ToggleGroupItem>
        {showPipeline && (
          <ToggleGroupItem
            value="pipeline"
            className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
          >
            <IconTimeline className={mobileControlIconClass} />
            {t("kanban:pipeline")}
          </ToggleGroupItem>
        )}
        <ToggleGroupItem
          value="threads"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconColumns className={mobileControlIconClass} />
          {t("kanban:threads")}
        </ToggleGroupItem>
        <ToggleGroupItem
          value="list"
          className="h-10 min-w-0 flex-1 cursor-pointer gap-2 text-sm data-[state=on]:bg-muted data-[state=on]:text-foreground"
        >
          <IconList className={mobileControlIconClass} />
          {t("kanban:list")}
        </ToggleGroupItem>
      </ToggleGroup>
    </div>
  );
}

function ResponsiveMenuSurface({
  isMobile,
  listingOnly,
  open,
  onOpenChange,
  contentRef,
  onOpenAutoFocus,
  onCloseAutoFocus,
  children,
}: {
  isMobile: boolean;
  listingOnly?: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  contentRef: RefObject<HTMLDivElement | null>;
  onOpenAutoFocus: (event: Event) => void;
  onCloseAutoFocus?: (event: Event) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  if (isMobile) {
    return (
      <MobileConfirmationHost open={open} surface="drawer">
        {({ contentProps }) => (
          <Drawer open={open} onOpenChange={onOpenChange}>
            <DrawerContent
              {...contentProps}
              ref={contentRef}
              tabIndex={-1}
              onOpenAutoFocus={onOpenAutoFocus}
              onCloseAutoFocus={onCloseAutoFocus}
              className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] !max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] outline-none"
            >
              <MobileConfirmationHostBody>
                <div
                  data-testid="mobile-home-menu-card"
                  className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl bg-background shadow-2xl shadow-black/20"
                >
                  <DrawerHeader className="shrink-0 border-b border-border/70 pb-3 text-left">
                    <DrawerTitle>
                      {t(listingOnly ? "common:viewOptions" : "kanban:menu")}
                    </DrawerTitle>
                  </DrawerHeader>
                  <div
                    data-testid="mobile-home-menu-scroll"
                    className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-[env(safe-area-inset-bottom,0px)]"
                  >
                    {children}
                  </div>
                </div>
              </MobileConfirmationHostBody>
            </DrawerContent>
          </Drawer>
        )}
      </MobileConfirmationHost>
    );
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        ref={contentRef}
        side="right"
        tabIndex={-1}
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={onCloseAutoFocus}
        className="w-full overflow-y-auto outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 sm:max-w-sm"
      >
        <SheetHeader>
          <SheetTitle>{t("kanban:menu")}</SheetTitle>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
}

function MobileMenuContent({
  isMobile,
  open,
  workspaceId,
  searchQuery,
  onSearchChange,
  isSearchLoading,
  onOpenChange,
  viewValue,
  onViewChange,
  showPipeline,
  displayOptions,
  navControls,
  pageActions,
  listingOnly,
  listingControls,
}: Pick<
  MobileMenuSheetProps,
  | "workspaceId"
  | "searchQuery"
  | "onSearchChange"
  | "isSearchLoading"
  | "onOpenChange"
  | "pageActions"
  | "listingOnly"
  | "listingControls"
> & {
  isMobile: boolean;
  open: boolean;
  viewValue: string;
  onViewChange: (value: string) => void;
  showPipeline: boolean;
  displayOptions: MobileDisplayOptionsProps;
  navControls: AppNavDialogControls;
}) {
  return (
    <div className="flex min-h-full flex-col gap-6 p-4">
      {pageActions ?? (
        <MobileSearchSection
          searchQuery={searchQuery ?? ""}
          onSearchChange={onSearchChange}
          isSearchLoading={isSearchLoading ?? false}
        />
      )}
      {!listingOnly && <MobileWorkspaceSection onOpenChange={onOpenChange} />}
      <MobileViewSection
        viewValue={viewValue}
        onViewChange={onViewChange}
        showPipeline={showPipeline}
      />
      <MobileDisplayOptions open={open} {...displayOptions} />
      {listingControls && (
        <MobileListingOptionsContext.Provider value={{ close: () => onOpenChange(false) }}>
          {listingControls}
        </MobileListingOptionsContext.Provider>
      )}
      {/* Phone Home lives in this menu; the View toggle owns listing modes. */}
      {!listingOnly && (
        <AppNavSections
          onNavigate={() => onOpenChange(false)}
          omitSections={isMobile || viewValue === "threads" ? [] : ["primary"]}
          omitDestinations={["tasks", "threads"]}
          workspaceActions={<MobileWorkspaceActionsSection workspaceId={workspaceId} />}
          controls={navControls}
        />
      )}
    </div>
  );
}

export function MobileMenuSheet({
  open,
  onOpenChange,
  onCloseAutoFocus,
  workspaceId,
  currentPage = "kanban",
  searchQuery = "",
  onSearchChange,
  isSearchLoading = false,
  tasksListOptions,
  pageActions,
  listingOnly = false,
  listingControls,
}: MobileMenuSheetProps) {
  const navControls = useAppNavDialogs(() => onOpenChange(false));
  const { contentRef, isMobile, viewValue, handleViewChange, displayOptions, focusMenu } =
    useMobileMenuSheetState({ open, onOpenChange, workspaceId, currentPage, tasksListOptions });

  return (
    <MobileMenuRender
      isMobile={isMobile}
      open={open}
      onOpenChange={onOpenChange}
      contentRef={contentRef}
      onOpenAutoFocus={focusMenu}
      onCloseAutoFocus={(event) => {
        onCloseAutoFocus?.(event);
        navControls.onMenuCloseAutoFocus?.(event);
      }}
      workspaceId={workspaceId}
      searchQuery={searchQuery}
      onSearchChange={onSearchChange}
      isSearchLoading={isSearchLoading}
      viewValue={viewValue}
      onViewChange={handleViewChange}
      displayOptions={displayOptions}
      navControls={navControls}
      pageActions={pageActions}
      listingOnly={listingOnly}
      listingControls={listingControls}
    />
  );
}

function MobileMenuRender(
  props: Pick<
    MobileMenuSheetProps,
    | "open"
    | "onOpenChange"
    | "onCloseAutoFocus"
    | "workspaceId"
    | "searchQuery"
    | "onSearchChange"
    | "isSearchLoading"
    | "pageActions"
    | "listingOnly"
    | "listingControls"
  > & {
    isMobile: boolean;
    contentRef: RefObject<HTMLDivElement | null>;
    onOpenAutoFocus: (event: Event) => void;
    viewValue: string;
    onViewChange: (value: string) => void;
    displayOptions: MobileDisplayOptionsProps;
    navControls: AppNavDialogControls;
  },
) {
  const { isMobile, navControls } = props;
  return (
    <>
      <ResponsiveMenuSurface {...props} isMobile={isMobile}>
        <MobileMenuContent {...props} showPipeline={!isMobile} />
      </ResponsiveMenuSurface>
      {navControls.dialogs}
    </>
  );
}
