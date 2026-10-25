# UI patterns

Design the first useful view for a narrow mobile viewport and expand it for
desktop. Use a clear page title, one primary action, readable empty states,
and a compact status area.

Use semantic headings and controls. Every icon-only control needs an accessible
name. Keep focus visible and preserve focus after a dialog closes. Support
keyboard activation for actions that also have a pointer gesture.

For asynchronous work, show loading, success, error, and retry states. Avoid
blocking the whole page for a small update. Confirm destructive operations and
state what will change before the user accepts.

## Appearance

Use the semantic CSS variables from the host appearance message when they are
available: `--background`, `--foreground`, `--card`, `--card-foreground`,
`--muted`, `--muted-foreground`, `--border`, `--primary`,
`--primary-foreground`, `--accent`, `--accent-foreground`, `--destructive`,
`--destructive-foreground`, and `--ring`.

Define usable light and dark fallback values in the document before the first
message. Accept only bounded color values from the parent window. Do not use
the appearance message for identity, permissions, data, navigation, or
actions.
