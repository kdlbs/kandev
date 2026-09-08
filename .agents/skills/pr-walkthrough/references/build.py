#!/usr/bin/env python3
"""Render a PR walkthrough HTML page from a JSON data file.

Usage:
    python3 build.py <data.json> [output.html]

The model writes only the JSON. This script owns the fixed shell, HTML
escaping, the dual canvas/list views, canvas node layout, GitHub file
anchors, and required-field validation. It fails loudly on a missing field,
so a walkthrough is never silently incomplete.
"""
import re
import sys
import json
import html
import hashlib
from pathlib import Path
from urllib.parse import urlparse

HERE = Path(__file__).resolve().parent
SHELL = HERE / "shell.html"

# ---- canvas layout constants (canvas pixels) ----
NODE_W = 340
# Node column stride (node width + gap). The gap must stay wide enough for an
# edge label pill to sit in the clear space between two adjacent group boxes:
# clear gap = COL_STRIDE - NODE_W - 2*GROUP_PAD, kept around 120px so a
# nowrap label ("passes close") reads without overlapping either box.
COL_STRIDE = 500
ROW_STRIDE = 240       # node row stride
ORIGIN_X = 40
ORIGIN_Y = 60
GROUP_PAD = 20
GROUP_TOP_PAD = 28
NODE_EST_H = 150       # estimated node height for group-box sizing
STAGE_PAD = 120


class BuildError(Exception):
    pass


def esc(s):
    """Escape text for HTML body content."""
    return html.escape("" if s is None else str(s), quote=False)


def esc_attr(s):
    """Escape text for an HTML attribute value."""
    return html.escape("" if s is None else str(s), quote=True)


def require(cond, msg):
    if not cond:
        raise BuildError(msg)


def sha256_hex(s):
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def file_url(change, pr):
    """The GitHub link for a change: an explicit URL, or the diff anchor."""
    if change.get("file_url"):
        value = change["file_url"]
        parsed = urlparse(value)
        require(parsed.scheme in {"http", "https"} and bool(parsed.netloc),
                "file_url must be an HTTP or HTTPS URL")
        return value
    return f'{pr["url"]}/files#diff-{sha256_hex(change["file"])}'


# Comment token per Shiki language, for the "// [!code ++]" marker a raw
# patch is converted into. Falls back to "//" for anything not listed.
COMMENT_TOKENS = {
    "python": "#", "py": "#", "bash": "#", "sh": "#", "shell": "#",
    "shellscript": "#", "zsh": "#", "fish": "#", "ruby": "#", "rb": "#",
    "yaml": "#", "yml": "#", "toml": "#", "makefile": "#", "dockerfile": "#",
    "r": "#", "perl": "#", "elixir": "#",
    "sql": "--", "haskell": "--", "lua": "--",
    "lisp": ";", "clojure": ";", "scheme": ";",
}


def patch_to_marked(patch):
    """Turn a raw unified diff into clean code plus added/removed line indices.

    Each patch line drives the output: '+' becomes an added line, '-' a
    removed line, and a leading space is context. Diff headers (---, +++,
    @@, diff --git, index) are dropped. The +/- classification is returned as
    two lists of 1-based line indices, not as inline comment markers, so a
    changed line whose body contains a comment token inside a string (for
    example a Rust `r#"...//..."#` literal) is still tagged correctly.

    File headers (---, +++, diff , index ) only appear before the first @@ hunk,
    so they are stripped only while not yet inside a hunk. Once a hunk starts,
    every line is classified by its first character. This keeps content lines
    whose body starts with "--" (a removed "-- comment") or "++" (an added
    "++i") instead of mistaking them for headers and dropping them.

    Returns (code, added, removed): the joined source, and two 1-based index
    lists for the added and removed lines.
    """
    out, added, removed = [], [], []
    in_hunk = False
    for raw in patch.splitlines():
        if raw.startswith("@@"):
            in_hunk = True
            continue
        if not in_hunk and raw.startswith(("+++", "---", "diff ", "index ")):
            continue
        if not raw:
            out.append("")
            continue
        tag, body = raw[0], raw[1:]
        if tag == "+":
            out.append(body)
            added.append(len(out))
        elif tag == "-":
            out.append(body)
            removed.append(len(out))
        elif tag == "\\":
            # "\ No newline at end of file" is a git marker, not source.
            continue
        else:
            out.append(body if tag == " " else raw)
    return "\n".join(out), added, removed


# Line-risk severities that get a colored heatmap bar. "low" is accepted in
# the schema but draws no bar (it is not a concern the reviewer must see).
RISK_SEVERITIES = ("low", "medium", "high")
RISK_TINTED = ("medium", "high")


def validate_block_risk(risk, ctx):
    """Check an optional per-block "risk" object. Return it unchanged."""
    require(isinstance(risk, dict), f'{ctx}: "risk" must be an object')
    if risk.get("score") is not None:
        s = int(risk["score"])
        require(1 <= s <= 10, f'{ctx}: risk.score must be 1-10')
    for j, ln in enumerate(risk.get("lines") or []):
        lctx = f'{ctx}: risk.lines[{j}]'
        require(ln.get("match"), f'{lctx}.match is required')
        sev = (ln.get("severity") or "").lower()
        require(sev in RISK_SEVERITIES,
                f'{lctx}.severity must be one of {RISK_SEVERITIES}')
    return risk


def block_risk_attr(risk):
    """A JSON data-risk attribute the page JS reads after Shiki renders.

    Only the fields the JS needs (lines: match/severity/note) are carried;
    the score/reason drive the chip, which build.py renders directly.
    """
    lines = [
        {"match": ln["match"],
         "severity": (ln.get("severity") or "").lower(),
         "note": ln.get("note", "")}
        for ln in (risk.get("lines") or [])
    ]
    if not lines:
        return ""
    return f" data-risk='{esc_attr(json.dumps(lines))}'"


def block_risk_chip(risk):
    """A colored risk chip for the block label row, colored client-side.

    Carries data-score so initRisk's shared riskStyle colors it with the
    same thresholds as the risk section and the topbar badge.
    """
    if risk.get("score") is None:
        return ""
    score = int(risk["score"])
    reason = risk.get("reason", "")
    title = f' title="{esc_attr(reason)}"' if reason else ""
    return (f'<span class="block-risk-chip" data-score="{esc_attr(score)}"'
            f'{title}><span class="block-risk-word">Risk</span> '
            f'<span class="block-risk-score">{esc(score)}</span>'
            f'<span class="block-risk-level"></span></span>')


# A hand-marked diff line ends with a "[!code ++]" or "[!code --]" comment.
# The comment token before the marker is optional and any token is accepted,
# so the same regex works for every language.
_MARK_ADD = re.compile(r"\s*(?://|#|--|;)?\s*\[!code \+\+\]\s*$")
_MARK_DEL = re.compile(r"\s*(?://|#|--|;)?\s*\[!code --\]\s*$")


def parse_hand_marked(code):
    """Split hand-marked diff code into clean code plus added/removed indices.

    A line ending with "[!code ++]" (added) or "[!code --]" (removed) is
    stripped of its marker and its 1-based index is recorded. This removes the
    marker at build time, so a marker inside a string never survives to the
    page. Returns (code, added, removed).
    """
    out, added, removed = [], [], []
    for ln in code.splitlines():
        if _MARK_ADD.search(ln):
            out.append(_MARK_ADD.sub("", ln))
            added.append(len(out))
        elif _MARK_DEL.search(ln):
            out.append(_MARK_DEL.sub("", ln))
            removed.append(len(out))
        else:
            out.append(ln)
    return "\n".join(out), added, removed


def all_added_indices(code):
    """Return the 1-based index of every line in an entirely-new block.

    Used for a block that is entirely new in the PR: the agent sets
    "diff": true but marks no lines, so every line reads as added. Blank lines
    are included too, so the green background stays continuous across a blank
    line in the middle of the excerpt instead of leaving an untinted gap.
    """
    return list(range(1, len(code.splitlines()) + 1))


def diff_attrs(added, removed):
    """The data attributes the page JS reads to tint diff lines by index."""
    out = ' data-diff="true"'
    if added:
        out += f' data-added="{",".join(str(i) for i in added)}"'
    if removed:
        out += f' data-removed="{",".join(str(i) for i in removed)}"'
    return out


def render_block(block, ctx):
    """One code block: a Shiki excerpt, a diff, or rendered Markdown."""
    if block.get("render") == "markdown":
        require(block.get("code") is not None, f'{ctx}: block missing "code"')
        lang = block.get("lang", "markdown")
        return (f'<pre class="code-block" data-lang="{esc_attr(lang)}" '
                f'data-render="markdown"><code>{esc(block["code"])}</code></pre>')
    require(block.get("lang"), f'{ctx}: block missing "lang"')
    added, removed = [], []
    if block.get("patch") is not None:
        require(block.get("code") is None,
                f'{ctx}: set "patch" or "code", not both')
        require(not block.get("diff"),
                f'{ctx}: "patch" already implies a diff; drop "diff"')
        code, added, removed = patch_to_marked(block["patch"])
        is_diff = True
    else:
        require(block.get("code") is not None, f'{ctx}: block missing "code"')
        code, is_diff = block["code"], block.get("diff")
        if is_diff and "[!code " in code:
            # Hand-marked lines: strip the markers and record their indices.
            code, added, removed = parse_hand_marked(code)
        elif is_diff:
            # A diff block with no marker is a fully-added excerpt, so every
            # non-blank line reads as added and the block tints green.
            added = all_added_indices(code)
    diff = diff_attrs(added, removed) if is_diff else ""
    risk_attr = block_risk_attr(block["risk"]) if block.get("risk") else ""
    return (f'<pre class="code-block" data-lang="{esc_attr(block["lang"])}"'
            f'{diff}{risk_attr}><code>{esc(code)}</code></pre>')


def layout(changes):
    """Assign an id and (x, y) to each change. Return group boxes.

    When changes carry a "group", each group is one column of nodes and gets
    a boundary box. Otherwise the nodes fall into a 2 or 3 column grid.
    """
    for i, c in enumerate(changes):
        c.setdefault("id", f"n{i + 1}")
    if any(c.get("group") for c in changes):
        order, cols = [], {}
        for c in changes:
            g = c.get("group") or ""
            if g not in cols:
                cols[g] = []
                order.append(g)
            cols[g].append(c)
        boxes = []
        for col_idx, g in enumerate(order):
            for row_idx, c in enumerate(cols[g]):
                c["_x"] = ORIGIN_X + col_idx * COL_STRIDE
                c["_y"] = ORIGIN_Y + row_idx * ROW_STRIDE
            if g:
                rows = len(cols[g])
                boxes.append((
                    g,
                    ORIGIN_X + col_idx * COL_STRIDE - GROUP_PAD,
                    ORIGIN_Y - GROUP_TOP_PAD,
                    NODE_W + GROUP_PAD * 2,
                    (rows - 1) * ROW_STRIDE + NODE_EST_H + GROUP_PAD * 2,
                ))
        return boxes
    ncols = 3 if len(changes) > 4 else 2
    for i, c in enumerate(changes):
        c["_x"] = ORIGIN_X + (i % ncols) * COL_STRIDE
        c["_y"] = ORIGIN_Y + (i // ncols) * ROW_STRIDE
    return []


def stage_size(changes, boxes):
    """Size the stage so every node and box fits inside it."""
    max_x = max([c["_x"] + NODE_W for c in changes]
                + [b[1] + b[3] for b in boxes] + [0])
    max_y = max([c["_y"] + NODE_EST_H for c in changes]
                + [b[2] + b[4] for b in boxes] + [0])
    return max(2400, max_x + STAGE_PAD), max(1400, max_y + STAGE_PAD)


def block_label_row(b):
    """The label line above a code block, with an optional risk chip.

    Emitted when the block has a label or a risk chip; otherwise empty so a
    plain block keeps its old markup.
    """
    chip = block_risk_chip(b["risk"]) if b.get("risk") else ""
    label = b.get("label")
    if not label and not chip:
        return ""
    text = esc(label) if label else '<span class="block-risk-only">Code</span>'
    return f'<div class="cpanel-block-label">{text}{chip}</div>'


def render_cnode(c, pr):
    """One canvas node with its hidden detail block for the panel."""
    url = file_url(c, pr)
    file_link = (f'<a href="{esc_attr(url)}" target="_blank" rel="noopener" '
                 f'class="cnode-file">{esc(c["file"])} \u2197</a>') if url else ""
    blocks = "".join(
        block_label_row({**b, "label": b.get("label", "Change")})
        + render_block(b, f'change {c["id"]}')
        for b in c["blocks"]
    )
    group = c.get("group") or ""
    group_attr = f' data-group="{esc_attr(group)}"' if group else ""
    return (
        f'<div class="cnode" data-id="{esc_attr(c["id"])}"{group_attr} '
        f'style="left: {c["_x"]}px; top: {c["_y"]}px;">'
        f'<div class="cnode-head"><span class="cnode-title">{esc(c["title"])}</span>'
        f'{file_link}</div>'
        f'<div class="cnode-sig">{esc(c.get("sig", ""))}</div>'
        f'<div class="cnode-more">Click for details \u2192</div>'
        f'<div class="cnode-detail" data-title="{esc_attr(c["title"])}" '
        f'data-file="{esc_attr(c["file"])}" data-file-url="{esc_attr(url)}">'
        f'<p class="cpanel-desc">{esc(c["why"])}</p>{blocks}</div></div>'
    )


def render_article(c, pr):
    """The linear-list version of one change (plain-text fallback)."""
    url = file_url(c, pr)
    blocks = ""
    for b in c["blocks"]:
        blocks += block_label_row(b)
        blocks += render_block(b, f'change {c["id"]}')
    return (
        '<article class="mb-9">'
        f'<h3 class="font-semibold text-brand-soft mb-1">{esc(c["title"])}</h3>'
        f'<a href="{esc_attr(url)}" target="_blank" rel="noopener" '
        'class="inline-flex items-center gap-1 text-sm text-slate-500 '
        'dark:text-slate-400 font-mono mb-2 hover:text-brand">'
        f'{esc(c["file"])} <span aria-hidden="true">\u2197</span></a>'
        f'<p class="mb-3">{esc(c["why"])}</p>{blocks}</article>'
    )


def render_groups(boxes):
    # The initial left/top/width/height are estimates. sizeGroups() in the shell
    # recomputes each box from its member nodes' real heights after Shiki settles
    # them, keyed on data-group, so the dashed box always wraps its nodes.
    return "".join(
        f'<div class="cgroup" data-group="{esc_attr(name)}" '
        f'style="left: {left}px; top: {top}px; '
        f'width: {w}px; height: {h}px;">'
        f'<span class="cgroup-label"><span class="dot"></span>{esc(name)}</span></div>'
        for name, left, top, w, h in boxes
    )


def edge_lanes(edges):
    """Assign stable slots for endpoints and repeated node pairs."""
    outgoing, incoming, pairs = {}, {}, {}
    for index, edge in enumerate(edges):
        outgoing.setdefault(edge["from"], []).append(index)
        incoming.setdefault(edge["to"], []).append(index)
        pair = tuple(sorted((edge["from"], edge["to"])))
        pairs.setdefault(pair, []).append(index)
    lanes = []
    for index, edge in enumerate(edges):
        source_edges = outgoing[edge["from"]]
        target_edges = incoming[edge["to"]]
        pair_edges = pairs[tuple(sorted((edge["from"], edge["to"])))]
        lanes.append({
            "from_lane": source_edges.index(index),
            "from_lanes": len(source_edges),
            "to_lane": target_edges.index(index),
            "to_lanes": len(target_edges),
            "pair_lane": pair_edges.index(index),
            "pair_lanes": len(pair_edges),
        })
    return lanes


def render_edges(edges, ids):
    out = ""
    for e in edges:
        require(e.get("from") in ids, f'edge references unknown node "{e.get("from")}"')
        require(e.get("to") in ids, f'edge references unknown node "{e.get("to")}"')
    for e, lane in zip(edges, edge_lanes(edges)):
        lane_attrs = " ".join(
            f'data-{name.replace("_", "-")}="{value}"'
            for name, value in lane.items()
        )
        out += (f'<div class="cedge" data-from="{esc_attr(e["from"])}" '
                f'data-to="{esc_attr(e["to"])}" '
                f'data-label="{esc_attr(e.get("label", ""))}" '
                f'{lane_attrs}></div>')
    return out


def mermaid_box(src, extra=""):
    return (f'<div class="rounded-xl border border-slate-200 dark:border-white/10 '
            f'p-4 bg-slate-50/60 dark:bg-white/5{extra}">'
            f'<pre class="mermaid-src"><code>{esc(src)}</code></pre></div>')


def sec_tldr(pr):
    url = esc_attr(pr["url"])
    return f'''<section id="tldr" class="anchor-target mb-14">
      <h1 class="text-3xl font-bold tracking-tight"><a href="{url}" class="hover:text-brand">{esc(pr["title"])} <span class="text-slate-400 dark:text-slate-500 font-normal">\u2197</span></a></h1>
      <div class="mt-3 flex flex-wrap gap-2 text-xs">
        <span class="rounded-full bg-slate-100 dark:bg-white/10 px-2.5 py-1">{esc(pr["base"])} \u2190 {esc(pr["head"])}</span>
        <span class="rounded-full bg-slate-100 dark:bg-white/10 px-2.5 py-1">{esc(pr.get("files_changed", "?"))} files</span>
        <span class="rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300 px-2.5 py-1">+{esc(pr.get("added", "?"))}</span>
        <span class="rounded-full bg-rose-100 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300 px-2.5 py-1">\u2212{esc(pr.get("removed", "?"))}</span>
        <a href="{url}" class="rounded-full border border-slate-300 dark:border-white/15 px-2.5 py-1 hover:text-brand">PR #{esc(pr["number"])} \u2197</a>
      </div>
      <p class="mt-5 text-lg text-slate-600 dark:text-slate-300">{esc(pr.get("tldr", ""))}</p>
    </section>'''


def sec_why(why):
    items = "".join(f'<li>{esc(w)}</li>' for w in why["what"])
    return f'''<section id="why" class="anchor-target mb-14">
      <h2 class="text-xl font-semibold mb-3">Why this change</h2>
      <p class="mb-4">{esc(why["problem"])}</p>
      <h3 class="text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-2">What it does</h3>
      <ul class="list-disc pl-5 space-y-1.5 marker:text-brand">{items}</ul>
    </section>'''


def sec_arch(arch):
    return f'''<section id="architecture" class="anchor-target mb-14">
      <h2 class="text-xl font-semibold mb-3">Architecture, end to end</h2>
      <p class="mb-4 text-slate-600 dark:text-slate-300">{esc(arch.get("caption", ""))}</p>
      {mermaid_box(arch["mermaid"])}
    </section>'''


def sec_changes(changes, edges, pr):
    boxes = layout(changes)
    ids = {c["id"] for c in changes}
    sw, sh = stage_size(changes, boxes)
    stage_inner = (f'<svg id="canvas-edges" class="absolute left-0 top-0 '
                   f'pointer-events-none" width="{sw}" height="{sh}" '
                   f'style="overflow: visible;"></svg>'
                   + render_groups(boxes)
                   + "".join(render_cnode(c, pr) for c in changes)
                   + render_edges(edges, ids))
    articles = "".join(render_article(c, pr) for c in changes)
    return f'''<section id="changes" class="anchor-target mb-14">
      <h2 class="text-xl font-semibold mb-2">Key code changes</h2>
      <p class="mb-4 text-slate-600 dark:text-slate-300">Drag to pan. Use the + and \u2212 buttons to zoom. Click a node to open the full code. The arrows show how the parts interact.</p>
      <div id="canvas-wrap" class="canvas-full-bleed relative rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50/60 dark:bg-white/5 overflow-hidden" style="height: 640px; touch-action: none;">
        <div class="absolute right-3 top-3 z-20 flex gap-1">
          <button data-cz="out" class="w-8 h-8 rounded-lg border border-slate-300 dark:border-white/15 bg-white/90 dark:bg-white/10 text-lg leading-none hover:bg-slate-100 dark:hover:bg-white/20">\u2212</button>
          <button data-cz="in" class="w-8 h-8 rounded-lg border border-slate-300 dark:border-white/15 bg-white/90 dark:bg-white/10 text-lg leading-none hover:bg-slate-100 dark:hover:bg-white/20">+</button>
          <button data-cz="fit" class="h-8 px-2 rounded-lg border border-slate-300 dark:border-white/15 bg-white/90 dark:bg-white/10 text-xs hover:bg-slate-100 dark:hover:bg-white/20">Fit</button>
          <button data-cz="max" title="Maximize" aria-label="Maximize canvas" class="h-8 px-2 rounded-lg border border-slate-300 dark:border-white/15 bg-white/90 dark:bg-white/10 text-xs hover:bg-slate-100 dark:hover:bg-white/20"><span class="label-max">Maximize</span><span class="label-min" style="display:none">Close</span></button>
        </div>
        <p class="absolute left-3 bottom-2 z-20 text-[11px] text-slate-400 dark:text-slate-500 pointer-events-none">drag to pan \u00b7 +/\u2212 to zoom \u00b7 click a node for details</p>
        <aside id="canvas-panel" class="cpanel" aria-hidden="true">
          <div id="cpanel-resizer" class="cpanel-resizer" role="separator" aria-orientation="vertical" aria-label="Resize details panel"></div>
          <div class="cpanel-head">
            <div>
              <div id="cpanel-title" class="cpanel-title"></div>
              <a id="cpanel-file" class="cpanel-file" target="_blank" rel="noopener"></a>
            </div>
            <button id="cpanel-close" type="button" class="cpanel-close" aria-label="Close details">\u00d7</button>
          </div>
          <div id="cpanel-body" class="cpanel-body"></div>
        </aside>
        <div id="canvas-stage" class="absolute left-0 top-0 origin-top-left" style="width: {sw}px; height: {sh}px;">{stage_inner}</div>
      </div>
      <details class="mt-5 group">
        <summary class="cursor-pointer select-none text-sm text-slate-500 dark:text-slate-400 hover:text-brand">Read the changes as a list</summary>
        <div class="mt-5">{articles}</div>
      </details>
    </section>'''


def sec_data(data):
    parts = []
    if data.get("mermaid"):
        parts.append(mermaid_box(data["mermaid"], extra=" mb-6"))
    if data.get("fields"):
        rows = "".join(
            f'<tr><td class="px-4 py-2 font-mono">{esc(f.get("field", ""))}</td>'
            f'<td class="px-4 py-2 font-mono">{esc(f.get("type", ""))}</td>'
            f'<td class="px-4 py-2">{esc(f.get("note", ""))}</td></tr>'
            for f in data["fields"]
        )
        parts.append(
            '<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-white/10">'
            '<table class="w-full text-sm"><thead class="bg-slate-100 dark:bg-white/5 text-left">'
            '<tr><th class="px-4 py-2 font-semibold">Field</th>'
            '<th class="px-4 py-2 font-semibold">Type</th>'
            '<th class="px-4 py-2 font-semibold">Notes</th></tr></thead>'
            f'<tbody class="divide-y divide-slate-200 dark:divide-white/10">{rows}</tbody></table></div>'
        )
    return f'''<section id="data" class="anchor-target mb-14">
      <h2 class="text-xl font-semibold mb-3">Data and storage</h2>
      <p class="mb-4 text-slate-600 dark:text-slate-300">{esc(data.get("caption", ""))}</p>
      {"".join(parts)}
    </section>'''


def sec_risk(risk):
    score = int(risk["score"])
    level = "Low" if score <= 3 else "Medium" if score <= 6 else "High"
    reasons = "".join(f'<li>{esc(r)}</li>' for r in risk["reasons"])
    return f'''<section id="risk" class="anchor-target mb-14">
      <h2 class="text-xl font-semibold mb-3">Risk</h2>
      <div class="rounded-xl border border-slate-200 dark:border-white/10 p-5 bg-slate-50/60 dark:bg-white/5">
        <div class="flex items-baseline gap-3">
          <span class="text-4xl font-bold" id="risk-score">{score}</span>
          <span class="text-slate-500 dark:text-slate-400 text-lg">/ 10</span>
          <span id="risk-level" class="ml-auto rounded-full px-3 py-1 text-sm font-semibold">{level}</span>
        </div>
        <div id="risk-meter" data-score="{score}" class="mt-4 relative">
          <div class="risk-track"></div>
          <div class="absolute -top-1" id="risk-knob-wrap" style="left: 0%;"><div class="risk-knob"></div></div>
          <div class="mt-1 flex justify-between text-[11px] text-slate-400 dark:text-slate-500"><span>1 low</span><span>5 medium</span><span>10 high</span></div>
        </div>
        <h3 class="text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mt-6 mb-2">Why this score</h3>
        <ul class="list-disc pl-5 space-y-1.5 marker:text-brand">{reasons}</ul>
      </div>
    </section>'''


def sec_review(review):
    tradeoffs = "".join(f'<li>{esc(t)}</li>' for t in review.get("tradeoffs", []))
    focus = "".join(f'<li>{esc(f)}</li>' for f in review.get("focus", []))
    return f'''<section id="review" class="anchor-target mb-8">
      <h2 class="text-xl font-semibold mb-3">Trade-offs and review notes</h2>
      <ul class="list-disc pl-5 space-y-1.5 marker:text-brand">{tradeoffs}</ul>
      <h3 class="text-sm font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400 mt-6 mb-2">Where to look first</h3>
      <ol class="list-decimal pl-5 space-y-1.5">{focus}</ol>
    </section>'''


def render_content(data):
    pr = data["pr"]
    parts = [sec_tldr(pr), sec_why(data["why"])]
    if data.get("architecture"):
        parts.append(sec_arch(data["architecture"]))
    parts.append(sec_changes(data["changes"], data.get("edges", []), pr))
    if data.get("data"):
        parts.append(sec_data(data["data"]))
    parts.append(sec_risk(data["risk"]))
    parts.append(sec_review(data["review"]))
    return "\n\n    ".join(parts)


def require_str_list(value, ctx):
    """Require a non-empty list whose entries are all non-empty strings.

    A plain string passes a truthiness check but the renderer iterates it
    character by character into one <li> per letter, so the shape is checked
    here to fail loudly on malformed walkthrough data.
    """
    require(isinstance(value, list) and value,
            f"{ctx} needs at least one item")
    for k, item in enumerate(value):
        require(isinstance(item, str) and item.strip(),
                f"{ctx}[{k}] must be a non-empty string")


def validate(data):
    require("pr" in data, 'missing "pr"')
    pr = data["pr"]
    for k in ("number", "title", "url", "base", "head", "repo"):
        require(pr.get(k) not in (None, ""), f'pr.{k} is required')
    require(data.get("why", {}).get("problem"), "why.problem is required")
    require_str_list(data.get("why", {}).get("what"), "why.what")
    changes = data.get("changes") or []
    require(len(changes) >= 1, "changes needs at least one entry")
    for i, c in enumerate(changes):
        for k in ("title", "file", "why"):
            require(c.get(k), f'changes[{i}].{k} is required')
        require(c.get("blocks"), f'changes[{i}].blocks needs at least one block')
        for j, b in enumerate(c["blocks"]):
            if b.get("risk"):
                validate_block_risk(b["risk"], f'changes[{i}].blocks[{j}]')
    score = data.get("risk", {}).get("score")
    require(score is not None, "risk.score is required")
    try:
        score = int(score)
    except (TypeError, ValueError):
        require(False, "risk.score must be a number")
    require(1 <= score <= 10, "risk.score must be 1-10")
    require_str_list(data.get("risk", {}).get("reasons"), "risk.reasons")
    require("review" in data, 'missing "review"')


def build(data):
    validate(data)
    pr = data["pr"]
    content = render_content(data)
    shell = SHELL.read_text(encoding="utf-8")
    require("{{CONTENT}}" in shell, "shell.html has no {{CONTENT}} marker")
    files_url = pr.get("files_url") or f'{pr["url"]}/files'
    for token, value in (
        ("{{PR_TITLE}}", esc(pr["title"])),
        ("{{PR_URL}}", esc_attr(pr["url"])),
        ("{{PR_FILES_URL}}", esc_attr(files_url)),
        ("{{PR_NUMBER}}", esc_attr(pr["number"])),
        ("{{REPO}}", esc_attr(pr.get("repo", ""))),
        ("{{RISK_SCORE}}", esc_attr(int(data["risk"]["score"]))),
    ):
        shell = shell.replace(token, value)
    shell = shell.replace("{{CONTENT}}", content)
    leftover = [t for t in ("{{PR_TITLE}}", "{{PR_URL}}", "{{PR_FILES_URL}}",
                            "{{PR_NUMBER}}", "{{REPO}}", "{{RISK_SCORE}}",
                            "{{CONTENT}}")
                if t in shell]
    require(not leftover, f"unreplaced placeholder(s): {leftover}")
    return shell


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    data = json.loads(Path(argv[1]).read_text(encoding="utf-8"))
    out = Path(argv[2]) if len(argv) > 2 else Path(f'pr-{data["pr"]["number"]}.html')
    try:
        html_out = build(data)
    except BuildError as e:
        print(f"build failed: {e}", file=sys.stderr)
        return 1
    out.write_text(html_out, encoding="utf-8")
    print(f"wrote {out} ({len(html_out.splitlines())} lines)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
