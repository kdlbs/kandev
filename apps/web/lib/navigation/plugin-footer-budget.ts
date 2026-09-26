/**
 * Inline budget for plugin `insights`-section destinations in the desktop
 * sidebar footer. Exported so conformance tests derive their expectations
 * from it rather than hard-coding the digit — see spec.md#Why-3, which also
 * states this is a layout constant, not a contract with plugin authors, and
 * may move without renegotiating that spec.
 *
 * Split out of `app-sidebar-footer.tsx` (a JSX/React module) into its own
 * side-effect-free module so e2e specs — which run in Playwright's Node
 * context, not the app's React tree — can import the same constant instead
 * of re-hard-coding it. `app-sidebar-footer.tsx` re-exports this so existing
 * unit-test imports from that module are unaffected.
 */
export const MAX_INLINE_PLUGIN_FOOTER_ITEMS = 3;
