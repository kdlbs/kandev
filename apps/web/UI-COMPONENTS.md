## UI Components

**Shadcn Components:** Import from `@kandev/ui` package:

```typescript
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog } from "@kandev/ui/dialog";
```

**Do NOT** import from `@/components/ui/*` - always use `@kandev/ui` package.

- Always prefer native shadcn components over custom implementations. For async-status UI, do not rely on a changing button label as a live announcement: use one translated `role="status"` region, keep the button's accessible name stable, and override `@kandev/ui/spinner`'s default English `aria-label` with `t(...)` when it is announced (or use `aria-hidden` when another live status covers it).
- Check `apps/packages/ui/src/` for available components (pagination, table, dialog, etc.).
- For data tables, use `@kandev/ui/table` with TanStack Table; use shadcn Pagination components.
- Only create custom components when shadcn doesn't provide what's needed.

### Responsive and touch surfaces

- Use `hooks/use-responsive-breakpoint.ts` for application layout decisions. Its mobile boundary matches the sidebar's 768px `md` boundary, and it also models tablet, compact desktop, full desktop, and pointer precision; do not substitute the UI package's generic `useIsMobile` hook. Tablet is a coarse-pointer fallback between `md` and `lg`, so no fine-pointer width reports it — a spec that needs the tablet layout has to emulate touch.
- When a Tailwind visibility class gates the same surface the hook picks, both must use the same boundary. A `sm:` class paired with a hook-driven mobile branch leaves 640-767px in a state neither side renders.
- CSS visibility classes can leave the other viewport tree mounted. Gate live effects such as polling, subscriptions, timers, and measurements with the same responsive or pointer condition, and test both initialization orders when readiness can precede or follow hydration.
- Use `useTouchDrawer` when a hover/popover disclosure needs a coarse-pointer `Drawer` alternative. Width-based phone composition and pointer-based disclosure behavior are related but not interchangeable. Apply the 44px minimum to coarse-pointer hit areas, touch rows, and mobile controls only; keep fine-pointer desktop controls at the surrounding design-system density, using 28px for ordinary buttons, inputs, and selectors and 24px only for deliberate compact inline controls, and do not reuse a touch-sized `h-11` class as the shared visual button size. In the expanded sidebar, the simple New Task button and labelled utilities use 28px controls; the compact New Task row uses 36px and adjacent Terminal/Quick Chat icon actions use 24px targets; coarse-pointer targets expand to at least 44px. See [navigation hierarchy](../../docs/specs/ui/requirements/navigation-hierarchy.md) and the [sizing guide](../../.agents/skills/mobile-parity/references/control-sizing.md) for other exceptions. Never float a text-selection action beside the selection on a coarse pointer: Android/iOS draw their native selection menu over it. Render `SelectionActionDock` (`components/shared/selection-action-dock.tsx`) docked to the surface's `data-selection-dock-host` scroller and follow the live selection with `useTouchSelectionTracking` (native handle drags fire only `selectionchange`). Code editors and the Markdown preview use `EditorSelectionCommentTrigger`. See [touch selection actions](../../docs/specs/ui/requirements/touch-selection-actions.md).
- Below 640px, shared Radix DropdownMenu/ContextMenu use inset, safe-area-aware bottom sheets in `app/globals.css`. Open `mobile-menu-root` content owns one decorative positioner backdrop matching Drawer dimming and blur; fade it with the sheet's exit motion. Never mark submenus; keep the backdrop outside scrolling content and pointer-transparent so Radix owns dismissal and non-modal interaction. Reuse these primitives and cover long/nested menus instead of adding parallel mobile menus.
- Mobile capability parity does not require desktop layout parity. Load `/mobile-parity` for the Kandev surface decision guide, mobile design contract, and verification requirements; read [confirmation guidance](components/confirmation/AGENTS.md) when adopting phone confirmation surfaces.
- Phone navigation uses `AppNavSheet`; the task title opens `ResponsiveTaskPicker`. Keep task actions and dialogs owned above removable responsive branches. Preserve saved tool order, workspace selection, focus restoration, and desktop preferences. For phone navigation, listing options, plugin controls, and collapsed integrations, follow the [mobile navigation design](../../docs/specs/ui/system-design/unified-mobile-navigation.md).
- Sidebar presentation preferences belong to the account; workspace layouts own entry visibility, order, and navigation sizing. `useSidebarCustomization` serializes immediate writes under layout revisions; the settings editor keeps Save/Discard drafts. Phone customization uses move controls and never writes desktop geometry or composition. For defaults, migration, and disclosure ordering, follow the [sidebar customization design](../../docs/specs/ui/system-design/sidebar-customization.md).
