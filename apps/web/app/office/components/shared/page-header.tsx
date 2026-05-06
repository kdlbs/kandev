interface PageHeaderProps {
  /**
   * Display title for the page. No longer rendered here — the office
   * topbar shows the canonical title for each route. Kept on the API
   * so existing call-sites stay compiling; treat it as documentation.
   */
  title?: string;
  action?: React.ReactNode;
}

/**
 * Page action row. The topbar owns the page title, so this slot only
 * renders the right-aligned action (e.g. "New Agent"). Returns null
 * when there's no action to keep DOM clean.
 */
export function PageHeader({ action }: PageHeaderProps) {
  if (!action) return null;
  return <div className="flex items-center justify-end">{action}</div>;
}
