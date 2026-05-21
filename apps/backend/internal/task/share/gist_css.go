package share

// shareCSS is the inlined stylesheet for the rendered share.html page.
// Self-contained — no @import, no external fonts — so the page renders
// identically on a fresh CDN with no network outside itself.
//
// Color tokens mirror kandev's dark-mode palette
// (apps/packages/theme/src/globals.css). We use the same oklch() values
// so the share page reads as part of the product rather than a separate
// site. oklch() has full support in current Chrome/Safari/Firefox.
//
// Layout: classic chat. User bubbles right-aligned with the primary
// (purple) fill; assistant content left-aligned in the card surface;
// system messages centered and de-emphasised. Tool calls render inline
// as collapsed pills inside the assistant bubble. Diffs are full-width
// code blocks with per-line tinting for adds/dels/hunks.
const shareCSS = `
:root {
  color-scheme: dark;

  /* — Kandev dark-mode tokens (oklch, ported from globals.css) — */
  --bg: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.205 0 0);
  --muted: oklch(0.269 0 0);
  --muted-foreground: oklch(0.708 0 0);
  --border: oklch(1 0 0 / 10%);
  --border-strong: oklch(1 0 0 / 18%);
  --primary: oklch(0.59 0.2 277);
  --primary-foreground: oklch(0.96 0.02 272);
  --accent: oklch(0.62 0.18 276);
  --accent-strong: oklch(0.79 0.1 275);
  --success: oklch(0.68 0.12 150);
  --destructive: oklch(0.704 0.191 22.216);

  /* — Derived semantic aliases used in the share layout — */
  --text: var(--foreground);
  --text-dim: var(--muted-foreground);
  --text-faint: oklch(0.55 0 0);
  --surface: var(--card);
  --surface-2: oklch(0.235 0 0);

  --diff-add-bg: color-mix(in oklab, var(--success) 14%, transparent);
  --diff-add-text: oklch(0.85 0.14 150);
  --diff-del-bg: color-mix(in oklab, var(--destructive) 14%, transparent);
  --diff-del-text: oklch(0.82 0.16 22);
  --diff-hunk: var(--accent-strong);

  --radius: 0.375rem;
  --radius-lg: 0.625rem;
  --radius-xl: 0.875rem;

  --mono: "Geist Mono", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  --sans: "Figtree", "Geist", ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  padding: 0;
  background: var(--bg);
  color: var(--text);
  font-family: var(--sans);
  font-size: 15px;
  line-height: 1.55;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

a { color: var(--accent-strong); text-decoration: none; }
a:hover { text-decoration: underline; }
code, pre { font-family: var(--mono); }
::selection { background: var(--primary); color: var(--primary-foreground); }

/* ─── Hero ─────────────────────────────────────────────────────── */

.hero {
  max-width: 880px;
  margin: 0 auto;
  padding: 56px 24px 28px;
  border-bottom: 1px solid var(--border);
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-dim);
  font-size: 13px;
  margin-bottom: 16px;
}
.brand a { color: var(--text); font-weight: 600; letter-spacing: -0.01em; }
.brand-sep { color: var(--text-faint); }
.brand-tag { color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.08em; font-size: 11px; }

.hero h1 {
  margin: 0 0 18px;
  font-size: 30px;
  font-weight: 600;
  letter-spacing: -0.015em;
  line-height: 1.2;
}

.badges { display: flex; flex-wrap: wrap; gap: 6px; }
.badge {
  display: inline-flex;
  align-items: center;
  padding: 3px 8px;
  border-radius: var(--radius);
  background: var(--surface);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 12px;
  font-family: var(--mono);
}

.redaction {
  margin: 16px 0 0;
  font-size: 13px;
  color: var(--text-dim);
}
.redaction code {
  padding: 1px 6px;
  background: var(--surface);
  border-radius: 4px;
  font-size: 12px;
}

/* ─── Conversation ────────────────────────────────────────────── */

.conv {
  max-width: 880px;
  margin: 0 auto;
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.group {
  display: flex;
  align-items: flex-end;
  gap: 10px;
}

.group-user { flex-direction: row-reverse; }
.group-assistant { flex-direction: row; }
.group-system { justify-content: center; }

.avatar {
  flex-shrink: 0;
  width: 30px;
  height: 30px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  margin-bottom: 4px;
}

.group-user .avatar {
  background: var(--primary);
  border-color: var(--primary);
}
.group-system .avatar { display: none; }

.bubble {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 16px;
  border-radius: var(--radius-xl);
  background: var(--surface);
  border: 1px solid var(--border);
}
.group-assistant .bubble {
  border-bottom-left-radius: var(--radius);
  flex: 1;
}
.group-user .bubble {
  background: var(--primary);
  color: var(--primary-foreground);
  border-color: var(--primary);
  border-bottom-right-radius: var(--radius);
  max-width: 75%;
}
.group-system .bubble {
  background: transparent;
  border-style: dashed;
  color: var(--text-dim);
  font-size: 13px;
  max-width: 480px;
  text-align: center;
}

/* Role label sits above the content. Hidden inside user bubbles since
   the right-alignment + color already convey "you said this." */
.role {
  font-size: 10.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-faint);
  margin: -2px 0 -4px;
}
.group-user .role { display: none; }
.group-assistant .role { color: var(--accent-strong); }

/* ─── Text inside bubbles ─────────────────────────────────────── */

.text > * { margin: 0; }
.text > * + * { margin-top: 8px; }
.text p { line-height: 1.6; }
.group-user .text { font-size: 15px; }

code.inline {
  background: color-mix(in oklab, var(--foreground) 8%, transparent);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 0.9em;
}
.group-user code.inline {
  background: color-mix(in oklab, white 22%, transparent);
  color: var(--primary-foreground);
}

pre.code {
  margin: 4px 0;
  padding: 12px 14px;
  border-radius: var(--radius-lg);
  background: var(--bg);
  border: 1px solid var(--border);
  overflow-x: auto;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text);
}
.group-user pre.code {
  background: color-mix(in oklab, black 25%, transparent);
  border-color: color-mix(in oklab, white 14%, transparent);
  color: var(--primary-foreground);
}

/* ─── Tool call / result pills ────────────────────────────────── */

.tool {
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: color-mix(in oklab, var(--foreground) 3%, transparent);
  overflow: hidden;
}
.tool > summary {
  list-style: none;
  cursor: pointer;
  padding: 7px 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-dim);
  user-select: none;
}
.tool > summary::-webkit-details-marker { display: none; }
.tool > summary:hover { background: color-mix(in oklab, var(--foreground) 4%, transparent); }

.tool-icon { font-size: 13px; }
.tool-name {
  color: var(--text);
  font-family: var(--mono);
  font-size: 12px;
}
.tool-summary {
  flex: 1;
  color: var(--text-faint);
  font-family: var(--mono);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.tool-chev {
  color: var(--text-faint);
  font-size: 10px;
  transition: transform 0.15s ease;
}
.tool[open] > summary .tool-chev { transform: rotate(90deg); }

.args, .output {
  margin: 0;
  padding: 12px 14px;
  background: var(--bg);
  border-top: 1px solid var(--border);
  font-size: 12.5px;
  line-height: 1.5;
  overflow-x: auto;
  white-space: pre;
  color: var(--text);
}

/* ─── Diff blocks ─────────────────────────────────────────────── */

.diff {
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--bg);
  overflow: hidden;
}
.diff-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
  color: var(--text-dim);
  background: var(--surface-2);
}
.diff-head code { color: var(--text); font-size: 12px; }
.diff-body {
  margin: 0;
  padding: 8px 0;
  font-size: 12.5px;
  line-height: 1.5;
  overflow-x: auto;
}
.diff-body > span {
  display: block;
  padding: 0 14px;
  white-space: pre;
}
.diff-add { background: var(--diff-add-bg); color: var(--diff-add-text); }
.diff-del { background: var(--diff-del-bg); color: var(--diff-del-text); }
.diff-hunk { color: var(--diff-hunk); }
.diff-file { color: var(--text-faint); }

/* ─── Misc ────────────────────────────────────────────────────── */

.empty {
  text-align: center;
  color: var(--text-faint);
  padding: 48px 0;
}

.page-footer {
  max-width: 880px;
  margin: 0 auto;
  padding: 32px 24px 48px;
  border-top: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-faint);
}
.cta {
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border-radius: var(--radius);
  background: var(--primary);
  color: var(--primary-foreground) !important;
  font-weight: 500;
}
.cta:hover { text-decoration: none; background: var(--accent); }
.foot-sep { color: var(--text-faint); }
.foot-link { color: var(--text-dim); }
.foot-version { color: var(--text-faint); font-family: var(--mono); font-size: 12px; }

/* ─── Mobile ──────────────────────────────────────────────────── */

@media (max-width: 640px) {
  .hero { padding: 32px 16px 20px; }
  .hero h1 { font-size: 22px; }
  .conv { padding: 20px 12px; gap: 18px; }
  .group { gap: 8px; }
  .avatar { width: 26px; height: 26px; font-size: 14px; }
  .bubble { padding: 10px 12px; border-radius: var(--radius-lg); }
  .group-user .bubble { max-width: 85%; }
}
`
