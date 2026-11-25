import type { SidebarTaskQuery } from "@/lib/types/http";

// A complete snapshot has at most 1,000 compact value keys plus Untagged.
// Larger disclosure is recovery input, never the final page's client intent.
export function usesSavedSidebarDisclosure(query: SidebarTaskQuery) {
  return (
    Boolean(query.disclosure_view_id) &&
    (query.collapsed_group_keys.length > 1_001 ||
      new TextEncoder().encode(JSON.stringify(query.collapsed_group_keys)).byteLength > 128 * 1024)
  );
}
