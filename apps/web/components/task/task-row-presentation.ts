import {
  normalizeSidebarTaskRowPresentation,
  type SidebarTaskRowDetail,
  type SidebarTaskRowPresentation,
  type SidebarTaskRowTrailing,
} from "@/lib/state/slices/ui/sidebar-task-row-presentation";

export type ResolvedTaskRowPresentation = {
  detailsEnabled: boolean;
  detailOrder: SidebarTaskRowDetail[];
  showRelativeTime: boolean;
  showRepository: boolean;
  showPullRequestNumber: boolean;
  trailing: SidebarTaskRowTrailing;
};

/** Resolves saved row settings against view-specific repository grouping. */
export function resolveTaskRowPresentation(
  value: SidebarTaskRowPresentation | undefined,
  options: { showRepository: boolean },
): ResolvedTaskRowPresentation {
  const presentation = normalizeSidebarTaskRowPresentation(value);
  const visible = new Set(presentation.visibleDetails);
  const detailOrder = presentation.detailOrder.filter((detail) => visible.has(detail));
  const detailsEnabled = presentation.detailsEnabled;

  return {
    detailsEnabled,
    detailOrder,
    showRelativeTime:
      detailsEnabled && visible.has("relative_time") && presentation.trailing !== "relative_time",
    showRepository: detailsEnabled && options.showRepository && visible.has("repository"),
    showPullRequestNumber: detailsEnabled && visible.has("pull_request_number"),
    trailing: presentation.trailing,
  };
}
