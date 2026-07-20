import { IconExternalLink } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

/**
 * Guard for rendering a plugin-supplied repo URL as an `<a href>`. A plugin's
 * repo_url comes from its manifest (installed plugins) or an untrusted
 * index.json (marketplace catalog), so only http(s) is allowed — a
 * `javascript:` (or other) scheme would execute in the operator's session on
 * click, and `rel`/`target` do NOT block it.
 */
export function isHttpUrl(url: string): boolean {
  return /^https?:\/\//i.test(url.trim());
}

/**
 * A guarded external "Repo" link to a plugin's source repository. Renders
 * nothing when the URL is empty or not http(s). Shared by the marketplace
 * catalog card and the installed-plugin list/detail so the scheme guard and
 * link markup live in one place.
 */
export function PluginRepoLink({ url, className }: { url: string; className?: string }) {
  if (!isHttpUrl(url)) return null;
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      data-testid="plugin-repo-link"
      className={cn(
        "inline-flex items-center gap-1 hover:text-foreground cursor-pointer",
        className,
      )}
    >
      <IconExternalLink className="h-3.5 w-3.5" />
      Repo
    </a>
  );
}
