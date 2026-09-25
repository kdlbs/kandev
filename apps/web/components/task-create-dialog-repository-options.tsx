import { useRef, useState, type ReactNode, type RefObject } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverTrigger, PopoverContent } from "@kandev/ui/popover";
import {
  Drawer,
  DrawerTrigger,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { useRepositoryCheckoutCapabilities } from "@/hooks/domains/workspace/use-repository-checkout-capabilities";
import type { RepositoryCheckoutOptions } from "@/lib/types/repository-checkout-options";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import {
  hasCustomCheckoutOptions,
  parseCheckoutDirectories,
} from "./task-create-dialog-checkout-options";
import { RepositoryOptionsFields } from "./task-create-dialog-repository-options-fields";

type Props = {
  row: TaskRemoteRepoRow;
  workspaceId?: string | null;
  executorProfileId?: string;
  onChange: (options?: RepositoryCheckoutOptions) => void;
};
export function RepositoryOptions({ row, workspaceId, executorProfileId, onChange }: Props) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [onDemand, setOnDemand] = useState(false);
  const [selectedFolders, setSelectedFolders] = useState(false);
  const [directories, setDirectories] = useState("");
  const [attempt, setAttempt] = useState(0);
  const state = useRepositoryCheckoutCapabilities(
    workspaceId,
    row.remoteUrl ?? row.url,
    row.provider,
    executorProfileId,
    { enabled: open, attempt },
  );
  const parsed = parseCheckoutDirectories(directories);
  const invalid = selectedFolders && (!!parsed.error || parsed.directories.length === 0);
  const unsupported =
    (onDemand && !state?.capabilities?.on_demand) ||
    (selectedFolders && !state?.capabilities?.sparse);
  const changeOpen = (value: boolean) => {
    if (value) {
      setOnDemand(row.checkoutOptions?.download_mode === "on_demand");
      setSelectedFolders(!!row.checkoutOptions?.sparse_directories.length);
      setDirectories(row.checkoutOptions?.sparse_directories.join("\n") ?? "");
    }
    setOpen(value);
  };
  const apply = () => {
    if (invalid || unsupported) return;
    onChange(
      onDemand || selectedFolders
        ? {
            version: 1,
            download_mode: onDemand ? "on_demand" : "standard",
            sparse_directories: selectedFolders ? parsed.directories : [],
          }
        : undefined,
    );
    setOpen(false);
  };
  const trigger = renderOptionsTrigger(row, triggerRef, t);
  const fields = (
    <RepositoryOptionsFields
      onDemand={onDemand}
      setOnDemand={setOnDemand}
      selectedFolders={selectedFolders}
      setSelectedFolders={setSelectedFolders}
      directories={directories}
      setDirectories={setDirectories}
      invalid={invalid}
      errorLines={parsed.errorLines}
      capabilities={state?.capabilities}
      failed={state?.failed}
      retry={() => setAttempt((value) => value + 1)}
    />
  );
  const footer = (
    <RepositoryOptionsFooter
      reset={() => {
        setOnDemand(false);
        setSelectedFolders(false);
        setDirectories("");
      }}
      cancel={() => setOpen(false)}
      apply={apply}
      disabled={invalid || unsupported}
    />
  );

  return (
    <RepositoryOptionsSurface
      open={open}
      changeOpen={changeOpen}
      trigger={trigger}
      fields={fields}
      footer={footer}
      row={row}
      triggerRef={triggerRef}
    />
  );
}

function RepositoryOptionsSurface({
  open,
  changeOpen,
  trigger,
  fields,
  footer,
  row,
  triggerRef,
}: {
  open: boolean;
  changeOpen: (value: boolean) => void;
  trigger: ReactNode;
  fields: ReactNode;
  footer: ReactNode;
  row: TaskRemoteRepoRow;
  triggerRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const container = useTaskCreateDialogPopoverContainer();
  const title = t("task:checkoutOptions.title");
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={changeOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          className="max-h-[85dvh]"
          data-testid="repository-options-drawer"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus();
          }}
        >
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription className="truncate">{row.fullName ?? row.url}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">{fields}</div>
          <div className="shrink-0 pb-[env(safe-area-inset-bottom)]">{footer}</div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Popover open={open} onOpenChange={changeOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        portalContainer={container}
        align="end"
        className="w-[320px] max-w-[calc(100vw-2rem)] p-0"
        data-testid="repository-options-popover"
      >
        <div className="max-h-[60vh] overflow-y-auto p-3">
          <h3 className="mb-3 text-xs font-medium">{title}</h3>
          {fields}
        </div>
        {footer}
      </PopoverContent>
    </Popover>
  );
}

export function RepositoryOptionsSummary({ options }: { options?: RepositoryCheckoutOptions }) {
  const { t } = useTranslation();
  if (!hasCustomCheckoutOptions(options)) return null;
  return (
    <span
      className="max-w-full text-xs text-muted-foreground"
      data-testid="repository-options-summary"
    >
      {options?.download_mode === "on_demand"
        ? t("task:checkoutOptions.onDemand")
        : t("task:checkoutOptions.standard")}
      {" · "}
      {options?.sparse_directories.length
        ? t("task:checkoutOptions.folderCount", { count: options.sparse_directories.length })
        : t("task:checkoutOptions.allFolders")}
    </span>
  );
}

function RepositoryOptionsFooter({
  reset,
  cancel,
  apply,
  disabled,
}: {
  reset: () => void;
  cancel: () => void;
  apply: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-2 border-t p-3">
      <Button
        type="button"
        variant="ghost"
        className="mr-auto h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
        onClick={reset}
      >
        {t("task:checkoutOptions.reset")}
      </Button>
      <Button
        type="button"
        variant="outline"
        className="h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
        onClick={cancel}
        data-testid="repository-options-cancel"
      >
        {t("task:checkoutOptions.cancel")}
      </Button>
      <Button
        type="button"
        className="h-7 cursor-pointer [@media(pointer:coarse)]:h-11"
        onClick={apply}
        disabled={disabled}
        data-testid="repository-options-apply"
      >
        {t("task:checkoutOptions.apply")}
      </Button>
    </div>
  );
}

function renderOptionsTrigger(
  row: TaskRemoteRepoRow,
  triggerRef: RefObject<HTMLButtonElement | null>,
  t: TFunction,
) {
  const title = t("task:checkoutOptions.title");
  return (
    <Button
      ref={triggerRef}
      type="button"
      variant="ghost"
      className="relative size-7 shrink-0 cursor-pointer p-0 [@media(pointer:coarse)]:size-11"
      aria-label={
        row.url
          ? t("task:checkoutOptions.openFor", { repository: row.fullName ?? row.url })
          : t("task:checkoutOptions.repositoryRequired")
      }
      title={row.url ? title : t("task:checkoutOptions.repositoryRequired")}
      disabled={!row.url}
      data-testid="repository-options-trigger"
    >
      <IconSettings className="size-3.5" />
      {hasCustomCheckoutOptions(row.checkoutOptions) && (
        <span className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
      )}
    </Button>
  );
}
