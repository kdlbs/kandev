import { useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronDown, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";

type PagePickerProps = {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
};

function PagePicker({ page, totalPages, onPageChange }: PagePickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selectedPage = useRef<HTMLButtonElement>(null);
  const currentPageId = useId();
  return (
    <Drawer open={open} onOpenChange={setOpen} autoFocus>
      <DrawerTrigger asChild>
        <Button
          variant="outline"
          className="min-w-0 flex-1 justify-between gap-2 text-sm tabular-nums"
          aria-label={t("github:choosePage")}
          aria-describedby={currentPageId}
        >
          <span id={currentPageId}>{t("github:pageOf", { page, total: totalPages })}</span>
          <IconChevronDown aria-hidden="true" />
        </Button>
      </DrawerTrigger>
      <DrawerContent
        className="data-[vaul-drawer-direction=bottom]:max-h-[80dvh] overflow-hidden"
        aria-describedby={undefined}
        onOpenAutoFocus={(event) => {
          if (!selectedPage.current) return;
          event.preventDefault();
          selectedPage.current?.focus({ preventScroll: true });
          selectedPage.current?.scrollIntoView({ block: "center" });
        }}
      >
        <DrawerHeader className="flex-row shrink-0 items-center justify-between px-3 py-2">
          <DrawerTitle>{t("github:choosePage")}</DrawerTitle>
          <DrawerClose asChild>
            <Button variant="ghost">{t("task:done")}</Button>
          </DrawerClose>
        </DrawerHeader>
        <div
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-[max(1rem,env(safe-area-inset-bottom))]"
          data-testid="github-page-picker-scroll"
        >
          {Array.from({ length: totalPages }, (_, index) => index + 1).map((value) => (
            <Button
              key={value}
              ref={value === page ? selectedPage : undefined}
              variant={value === page ? "secondary" : "ghost"}
              className="mb-1 h-11 w-full justify-between text-sm tabular-nums"
              aria-current={value === page ? "page" : undefined}
              onClick={() => {
                if (value !== page) onPageChange(value);
                setOpen(false);
              }}
            >
              {t("github:pageOf", { page: value, total: totalPages })}
              {value === page && <IconCheck aria-hidden="true" />}
            </Button>
          ))}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

export function MobileResultsPagination({ range, ...props }: PagePickerProps & { range: string }) {
  const { t } = useTranslation();
  const { page, totalPages, onPageChange } = props;
  return (
    <nav
      aria-label={t("github:choosePage")}
      className="shrink-0 border-t px-3 pt-2 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
      data-testid="github-results-pagination"
    >
      <p className="mb-2 text-center text-xs text-muted-foreground tabular-nums" aria-live="polite">
        {range}
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon"
          aria-label={t("github:previousPage")}
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <IconChevronLeft />
        </Button>
        <PagePicker {...props} />
        <Button
          variant="outline"
          size="icon"
          aria-label={t("github:nextPage")}
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          <IconChevronRight />
        </Button>
      </div>
    </nav>
  );
}
