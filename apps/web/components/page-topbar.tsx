import Link from "next/link";
import { IconArrowLeft } from "@tabler/icons-react";

type PageTopbarProps = {
  /** Page title shown in the breadcrumb */
  title: string;
  /** Optional subtitle shown to the right of the title */
  subtitle?: string;
  /** Optional icon rendered before the title */
  icon?: React.ReactNode;
  /** Where the back link navigates to (default: "/") */
  backHref?: string;
  /** Optional content rendered on the right side of the topbar */
  actions?: React.ReactNode;
};

export function PageTopbar({ title, subtitle, icon, backHref = "/", actions }: PageTopbarProps) {
  return (
    <header className="flex items-center gap-3 px-4 py-3 border-b shrink-0">
      <Link
        href={backHref}
        className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground cursor-pointer transition-colors"
      >
        <IconArrowLeft className="h-3.5 w-3.5" />
        KanDev
      </Link>
      <span className="text-muted-foreground/50">›</span>
      <div className="flex items-center gap-2">
        {icon}
        <span className="text-sm font-semibold">{title}</span>
        {subtitle && (
          <>
            <span className="text-muted-foreground/50">·</span>
            <span className="text-xs text-muted-foreground">{subtitle}</span>
          </>
        )}
      </div>
      {actions && <div className="ml-auto flex items-center gap-2">{actions}</div>}
    </header>
  );
}
