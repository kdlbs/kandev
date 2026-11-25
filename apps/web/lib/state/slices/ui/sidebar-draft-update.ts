import type { SidebarDraftUpdateApi } from "@/lib/types/http-user-settings";
import type { SidebarSnapshot } from "./sidebar-view-delta";
import { sidebarViewWithDraft } from "./sidebar-view-disclosure";
import { toSidebarViewUpdate } from "./sidebar-view-update";

export function sidebarDraftDelta(
  before: SidebarSnapshot,
  after: SidebarSnapshot,
): { draft_update?: SidebarDraftUpdateApi } {
  if (JSON.stringify(before.draft) === JSON.stringify(after.draft)) return {};
  if (!after.draft) {
    return before.draft
      ? {
          draft_update: {
            base_view_id: before.draft.baseViewId,
            expected_group: before.draft.group,
            clear: true,
          },
        }
      : {};
  }
  const id = after.draft.baseViewId;
  const base =
    before.views.find((view) => view.id === id) ?? after.views.find((view) => view.id === id);
  if (!base) {
    // i18n-exempt: programmer invariant; drafts cannot outlive their saved base.
    throw new Error("Sidebar draft base missing");
  }
  const previous = sidebarViewWithDraft(base, before.draft);
  const nextBase = after.views.find((view) => view.id === id) ?? base;
  const next = sidebarViewWithDraft(nextBase, after.draft);
  const {
    id: _id,
    name: _name,
    generated_plugin_grouping: _origin,
    ...fields
  } = toSidebarViewUpdate(previous, next) ?? { id };
  return { draft_update: { base_view_id: id, expected_group: previous.group, ...fields } };
}
