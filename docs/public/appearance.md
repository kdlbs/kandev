---
title: Appearance and themes
description: Import, preview and select themes for Kandev and its code viewers.
---

# Appearance and themes

Choose **Settings → Preferences → Appearance → Color theme** to preview a theme.
**Save changes** remembers it in this browser; **Reset** restores the saved choice.
System, Light and Dark remain available alongside installed package variants.
Themes can color the application, editors, diffs, highlighted code and terminals.

## Import a theme

An instance administrator can choose **Import theme** in Appearance or
**Settings → Plugins → Themes**. Upload a native theme `.tar.gz` package or a
standalone VS Code `.json` / `.jsonc` color theme. JSON comments and trailing
commas are accepted. If the file has no declared type, select Light or Dark.
Unscoped token foreground and background defaults fill missing editor colors;
explicit `colors.editor.*` values take precedence. Unscoped font styles appear
as unsupported in the compatibility report.

Choose **Review theme** to inspect variants, samples and compatibility findings,
then **Install themes**. Installation does not change anyone's selected theme.
Open Appearance, choose a variant and save it. Installed themes are available to
all authenticated users; each browser keeps its own selection.

A review expires after ten minutes. Review again if it expires or the installed
package changes before confirmation. Retrying a successful confirmation does not
install another copy.

## Browse and manage themes

The Plugins page's **Themes** tab searches theme listings from the configured
marketplace sources. Administrators can review and install a listing. Manage
sources in the existing plugin marketplace. Use the **Installed** tab to enable,
disable, update or remove a theme package, just like other packages. Manual and
automatic theme updates verify the selected release's kind, package ID, version
and SHA-256 digest before installation. A rejected update keeps the installed
version available.

If a saved theme becomes unavailable, Kandev uses its base appearance and keeps
the saved reference. Re-enable or reinstall the package to restore it, or select
a built-in theme. **Try again** refreshes inventory after a connection failure.

## Compatibility

Imports accept color data, not extension code, CSS, fonts, icons or external
includes. Unsupported VS Code roles use the base palette and appear in the review
report. TextMate rules work in Shiki and Pierre; portable syntax roles cover
Monaco and CodeMirror. Rendering can differ between languages and viewers.
Contrast findings are advisory; supplied colors are preserved.

After reload, the built-in scheme appears while authenticated theme data loads.
A fresh browser needs its own theme selection. To contribute a native package,
see [Theme authoring](themes-authoring.md).
