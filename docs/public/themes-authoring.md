---
title: Theme package reference
description: Define data-only Kandev themes and publish them through the existing marketplace.
---

# Theme package reference

Theme packages contribute palettes without running code. Create production
packages in their own repositories. The host validates the archive, its
checksums, manifest and every variant before installation or activation.

## Manifest

```yaml
id: example-colors
api_version: 2
version: 1.0.0
display_name: Example colors
author: Your name
categories: [theme]
distribution:
  schema_version: 1
  kind: theme
  license: MIT
themes:
  - key: night
    label: Night
    base: dark
    path: themes/night.json
```

A package contains 1–16 variants. Keys are lowercase letters, digits and hyphens,
up to 64 characters; keys and paths are unique. Each base is `light` or `dark`.
Variant paths must be clean relative JSON paths. Preserve attribution and include
the license covering your palette and any imported data.

Runtime endpoints, executables, frontend assets, permissions and other plugin
contributions are forbidden. Allowed files are the manifest, declared JSON
variants, checksums/signature, and optional README, LICENSE and NOTICE text files.

## Variant definition

```json
{
  "schema_version": 1,
  "base": "dark",
  "ui": { "background": "#272822", "foreground": "#f8f8f2" },
  "editor": { "background": "#272822", "selection": "#49483e" },
  "syntax": { "keyword": { "foreground": "#f92672" } },
  "terminal": { "green": "#a6e22e" }
}
```

The complete allowlist is the [version 1 JSON schema](https://github.com/kdlbs/kandev/blob/main/apps/backend/internal/plugins/themes/schema/v1.json),
also available from an authenticated instance at `/api/plugins/themes/schema/v1`.
Unknown fields are rejected. Colors are hexadecimal RGB/RGBA, including shorthand;
base surfaces must be opaque. Missing roles inherit the built-in base palette.
No CSS expressions, URLs or arbitrary property names are accepted.

| Group | Purpose |
| --- | --- |
| `ui` | Application semantic roles: background, foreground, card, sidebar, primary, border and others |
| `editor` | Editor colors, selection, gutter, diagnostics and added/removed diff colors |
| `syntax` | Portable comment, keyword, string, number, type, function and other token styles |
| `terminal` | Foreground/background, cursor, selection and ANSI colors |
| `textmate` | Bounded TextMate scope rules for Shiki and Pierre |
| `semantic` | Bounded semantic token selectors for compatible editor highlighting |

Token styles accept foreground, background and a space-separated `fontStyle`
using `italic`, `bold`, `underline` and `strikethrough`. Syntax support varies
between engines. Test both light/dark contrast and each viewer you intend to support.

Archives are limited to 8 MiB compressed and expanded, 128 files, 256 KiB per
variant and 64 KiB per metadata/document file. Links, duplicate paths, traversal,
undeclared files and checksum mismatches are rejected.

## Package and verify

From a Kandev checkout, use the same tools as plugin release CI:

```bash
cd apps/backend
go run ./cmd/plugin-pack -dir /path/to/theme -out /tmp/example-colors-1.0.0.tar.gz
go run ./cmd/plugin-package-verify --archive /tmp/example-colors-1.0.0.tar.gz \
  --expected-id example-colors --expected-version 1.0.0 --expected-kind theme
```

A reproducible two-variant example lives at
[`apps/backend/internal/plugins/themes/testdata/sample`](https://github.com/kdlbs/kandev/tree/main/apps/backend/internal/plugins/themes/testdata/sample).
It is a test fixture, not a bundled theme. Upload the resulting archive through
**Import theme**, inspect the review, install, and test both variants in Appearance.

## Publish to the marketplace

Publish an archive named `<id>-<version>.tar.gz` in a release of the theme's own
repository. Include its license and checksums, then submit a registry PR adding:

```yaml
- id: example-colors
  repo: your-name/example-colors
  kind: theme
  categories: [theme]
```

The index builder verifies the actual archive's kind, identity, version, license
and digest. Existing source precedence and update checks apply. See
[Marketplace contributions](plugins-marketplace.md) for source configuration and
curation. Publishing a listing is separate from importing it into your instance.

Theme updates must retain the theme package kind and match the selected listing's
ID, version and SHA-256 digest. A theme listing cannot replace an installed theme
with an executable plugin, including a package with a different ID.
