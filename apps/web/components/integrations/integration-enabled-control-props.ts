/**
 * Props every integration's enable/disable slider takes.
 *
 * The toggle is per workspace, so the slider always names the workspace it
 * writes. `undefined` means "no workspace resolved yet" and falls back to the
 * pre-scoping install-wide value; the settings routes that render these
 * sliders always know their workspace, so that path is only the brief moment
 * before one is resolved.
 */
export type IntegrationEnabledControlProps = {
  // Required (though it may be `undefined`) so a render site that forgets to
  // name its workspace fails to compile rather than silently editing another
  // workspace's toggle.
  workspaceId: string | undefined;
};
