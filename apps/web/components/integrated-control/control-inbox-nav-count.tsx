import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { useControlInbox } from "./control-inbox-provider";

/** The pending Needs-you count beside the control navigation's Inbox entry. */
export function ControlInboxNavCount({ collapsed }: { collapsed: boolean }) {
  const { t } = useTranslation();
  const inbox = useControlInbox();
  if (!inbox?.enabled || inbox.loading) return null;
  const { pending, partial } = inbox.counts;
  if (pending === 0 && !partial) return null;
  const label = partial
    ? t("sidebar:controlInbox.navCountPartial", { count: pending })
    : t("sidebar:controlInbox.navCount", { count: pending });
  return (
    <span
      className={cn(
        "inline-flex min-w-5 shrink-0 items-center justify-center rounded-full border border-warning/40 bg-background px-1.5 text-[11px] font-medium text-warning",
        collapsed && "sr-only",
      )}
      title={label}
      aria-label={label}
      data-testid="control-nav-inbox-count"
    >
      {partial ? `${pending}+` : pending}
    </span>
  );
}
