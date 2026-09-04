# Process

**Best for:** sequential business processes with multiple actors/divisions where the reader needs to see *who* does *what*, *what data* enters and leaves each step, and *which tools* are used — not just the step order. Covers responsibility audits, data-quality gate reviews, cross-divisional handoff maps, and end-to-end workflow documentation.

Prefer swimlane (simpler) when the data types and tools don't matter. Prefer process when each step's input/output payload and responsible team must be legible at a glance.

This type is **parametric** — the inputs schema in §1 drives every coordinate via the formulas in §2. Two generations from the same inputs must produce visually identical SVG. The rule shapes mirror `type-data-flow.md` so color override, IN/OUT chip semantic, and reproducibility checklist read identically across types.

---

## 1. Inputs — the parameter contract

```yaml
lanes:                              # 1..6 horizontal swimlanes (top to bottom)
  - { name: ["RD&E"],                 key: "RDE" }
  - { name: ["IT"],                   key: "IT"  }
  - { name: ["FIELD", "SERVICES"],    key: "FLD" }
  - { name: ["SURVEY", "SERVICES"],   key: "SVY" }
  - { name: ["HOUSEHOLD", "UNIT"],    key: "HHU" }
  - { name: ["COMMS &", "MARKETING"], key: "CMM" }

steps:                              # 1..12 vertical step columns (left to right)
  - { number: "1",  label: "Design"   }
  - { number: "2",  label: "Build"    }
  - { number: "3",  label: "Test", focal: true }       # focal step header chip — accent fill
  - { number: "4",  label: "Train"    }
  # ... up to 12

nodes:                              # explicit per-cell entries; empty cells render nothing
  - { lane: "RDE", step: 0,  title: "Survey design",      sub: "questionnaire · sampling", tool: "Excel · CSPro",
      chips: {in: null,  out: "FL"} }              # first step has no input chip
  - { lane: "IT",  step: 1,  title: "Build app",          sub: "form + validation",        tool: "CSPro · scripts",
      chips: {in: "FL", out: "TB"}, color: "#5a7d9a" }    # slate-blue — data quality concern
  - { lane: "RDE", step: 2,  title: "Pilot test",         sub: "field debug",              tool: "tablet · script",
      chips: {in: "TB", out: "TB"}, focal: true }   # focal node — accent border
  - { lane: "FLD", step: 3,  title: "Train enumerators",  sub: "protocols · safety",       tool: "manual",
      chips: {in: "TB", out: "LS"}, color: "#b85450" }    # rust-red — governance / training
  # ... etc

arrows:                             # explicit edges; styles bind to topology (see §3)
  - { from: {lane: "RDE", step: 0}, to: {lane: "IT",  step: 1}, style: "normal" }
  - { from: {lane: "IT",  step: 1}, to: {lane: "RDE", step: 2}, style: "focal-in" }     # accent — into focal
  - { from: {lane: "RDE", step: 2}, to: {lane: "FLD", step: 3}, style: "focal-out" }    # accent — out of focal
  - { from: {lane: "RDE", step: 2}, to: {lane: "IT",  step: 1}, style: "trigger" }      # dashed trigger
  # ... etc

dark: false
```

**Reserved field semantics:**
- `lanes[k].key` — the 3-letter role badge text shown inside every node in that lane.
- `lanes[k].name` — 1 or 2 line lane label; uppercase mono.
- `steps[j].focal: true` — exactly **one** step may declare this. Header chip renders in accent.
- `nodes[i].focal: true` — exactly **one** node may declare this. Renders with accent border (§5).
- `nodes[i].chips` — `{in: "<CODE>", out: "<CODE>"}` object (either side `null` to omit). Codes from §8. **Skip** the input chip on the first step's nodes, **skip** the output chip on the last step's nodes (no upstream / downstream).
- `nodes[i].color` — optional **per-node color override**. Any valid `"#hex"` string; the §4 palette is recommended for cross-diagram consistency.

---

## 2. Layout formulas — deterministic geometry

```
label_col_w      = 140
step_slot_w      = 112                                # 100-px node + 12-px corridor
right_pad        = 28
n_steps          = len(steps)
n_lanes          = len(lanes)

# Canvas
viewBox_w        = label_col_w + n_steps * step_slot_w + right_pad   # 11 steps → 1400
header_h         = 36
lane_h           = 80
has_color_row    = any(node.color or step.color or lane.color in inputs)
legend_h         = 100 if has_color_row else 80       # 4 rows when colors are present
viewBox_h        = header_h + n_lanes * lane_h + legend_h            # 6 lanes, no colors → 596; with → 616

# Header strip (top)
chip_y           = 8
chip_w           = 16                                  # 20 if step.number has 2 digits
chip_h           = 16
chip_rx          = 8                                   # pill

# Lane positions
lane_y_top(k)    = header_h + k * lane_h               # 36, 116, 196, 276, 356, 436
lane_y_mid(k)    = lane_y_top(k) + lane_h/2            # 76, 156, 236, 316, 396, 476
lane_label_x     = label_col_w / 2                     # 70

# Step / node centers
step_cx(j)       = label_col_w + 8 + j * step_slot_w + node_w/2      # 198, 310, 422, ...
                                                                      # (8-px gutter inside content area)

# Nodes
node_w           = 100
node_h           = 64
node_x(j)        = step_cx(j) - node_w/2
node_y(k)        = lane_y_top(k) + (lane_h - node_h)/2     # 8-px top/bottom margin inside lane

# Legend strip (bottom)
legend_y_top     = header_h + n_lanes * lane_h
legend_row_y     = [legend_y_top + 16, legend_y_top + 37,
                    legend_y_top + 58, legend_y_top + 79]
```

### 2.1 Background structure

- Paper fill across full viewBox.
- Dot pattern: 22×22 grid, `circle r=0.8`, `fill rgba(45,49,66,0.10)`. Opacity 0.55.
- Alternating lane tints: odd-indexed lanes (0, 2, …) receive `rgba(45,49,66,0.018)` fill from `x=140` to `viewBox_w`.
- Lane dividers: horizontal hairlines at every `lane_y_top(k)` and at `legend_y_top`; stroke `rgba(45,49,66,0.12)` width 0.8.
- Label column right border: vertical hairline at `x = label_col_w`, stroke `rgba(45,49,66,0.20)` width 1, from `y = header_h` to `y = legend_y_top`.

### 2.2 Step header chip + label

Per step `j`:

```
chip_w(j)        = 20 if len(step.number) >= 2 else 16
chip_x(j)        = step_cx(j) - chip_w(j)/2
number_anchor    = (step_cx(j), chip_y + 11)
label_anchor     = (step_cx(j), 32)              # 8-px gap below chip
```

**Chip** (the numbered pill at the top of each column):
- Default fill: `rgba(45,49,66,0.12)`, number text ink.
- Focal fill: `rgba(235,108,54,0.20)`, number text accent (§5).
- Per-step `color` override (§4): replaces the fill with `rgba(C, 0.20)` and the number fill with `C`.

**Label** (the uppercase mono text below the chip):
- Renders `steps[j].label` (uppercased), anchored at `label_anchor`.
- Font: Geist Mono 6 px, weight 500, `letter-spacing="0.12em"`, `text-anchor="middle"`.
- Default fill: muted (`#4f5d75` light / `#bfc0c0` dark).
- Focal fill: accent (`#eb6c36` light / `#f08a59` dark).
- Per-step `color` override: fill = `C` (matches the chip number color).
- Keep labels short (≤ 9 chars). Long labels truncate; if you need more, abbreviate.

### 2.3 Lane labels

One or two-line mono label, all uppercase, letter-spacing 0.08em, font-size 8, fill muted. Centered at `(lane_label_x, lane_y_mid(k))`:
- Single-line: anchored at `(lane_label_x, lane_y_mid(k) + 4)`
- Two-line: lines at `(lane_label_x, lane_y_mid(k) - 4)` and `(lane_label_x, lane_y_mid(k) + 4)`

Per-lane `color` override (§4): replaces the label fill with `C` and the lane stripe tint with `rgba(C, 0.04)`.

### 2.4 Node content layout (inside the 100×64 rect)

```
role_chip          rect 14×10 at (node_x+4, node_y+4),  rx=2
role_chip_text     centered at (node_x+11, node_y+12), font-size=6, weight=600
                                                                # text = lanes[k].key (3-letter lane code)
title              centered at (step_cx(j), node_y+26),  font-size=9 sans semibold
in→out             centered at (step_cx(j), node_y+40),  font-size=6.5 mono muted
tool               centered at (step_cx(j), node_y+52),  font-size=6.5 mono soft
data chip IN       rect 16×8 at (node_x+4,   node_y+54), rx=2      # payload entering
data chip OUT      rect 16×8 at (node_x+80,  node_y+54), rx=2      # payload leaving
```

**Role chip text rule:** the badge inside each node renders `lanes[k].key` where `k` is the node's lane index — **not** the step number (the step number already lives in the column header chip at the top, §2.2). Showing the lane key as the node badge gives each node a self-contained "who" identifier that survives when a single node is excerpted out of context. Mirrors the same rule in `type-data-flow.md` §2.4.

Empty cells (no node entry) render **nothing**. No placeholder rect, no role chip, no label.

**Chip-vs-tool-text collision rule:** chips sit at `node_y + 54..62`; tool text baseline is at `node_y + 52`. If a node has a two-line title (rare), increase node_h to 72 OR omit the chips for that node. Default behaviour: omit chips on collision.

---

## 3. Connector rules (mandatory)

Three styles, bound to topology. Connectors drawn **before** all node rects (z-order rule).

| `style` | Stroke | Width | Dash | Marker | When required |
|---|---|---|---|---|---|
| `normal` | `#4f5d75` (muted) | 1.0 | — | `arrow` | Standard data hand-off between steps or actors. Unlabelled. |
| `focal-in` / `focal-out` | `#eb6c36` (accent) | 1.2 | — | `arrow-accent` | Every edge whose endpoint is the focal node (`focal-in`) or origin is the focal node (`focal-out`). |
| `trigger` | `#4f5d75` (muted) | 1.0 | `4,3` | `arrow-sm` | Orchestration trigger (scheduler → tool, manual override → upstream step). Unlabelled. |

**Defs block** (required, three markers):

```svg
<defs>
  <pattern id="dots" width="22" height="22" patternUnits="userSpaceOnUse">
    <circle cx="11" cy="11" r="0.8" fill="rgba(45,49,66,0.10)"/>
  </pattern>
  <marker id="arrow"        markerWidth="8" markerHeight="6" refX="7" refY="3"   orient="auto"><polygon points="0 0, 8 3, 0 6" fill="#4f5d75"/></marker>
  <marker id="arrow-accent" markerWidth="8" markerHeight="6" refX="7" refY="3"   orient="auto"><polygon points="0 0, 8 3, 0 6" fill="#eb6c36"/></marker>
  <marker id="arrow-sm"     markerWidth="6" markerHeight="5" refX="5" refY="2.5" orient="auto"><polygon points="0 0, 6 2.5, 0 5" fill="#4f5d75"/></marker>
</defs>
```

### 3.1 Routing rules (non-negotiable)

**Single-bend right-angle:** exit RIGHT → corridor → enter TOP (↓ destination below) or BOTTOM (↑ destination above).

- Source-side: exit at `(node_x + 100, lane_y_mid(src_lane))` — node's right edge, vertical center.
- Destination-side: enter at `(step_cx(dst_step), node_y(dst_lane))` for downward, or `(step_cx(dst_step), node_y(dst_lane) + 64)` for upward.
- Corner radius: 8-px Q-bezier at the bend.
- Same-lane edges (rare — same lane, adjacent steps): horizontal `<line>` from src right to dst left.
- **No diagonals.** **No left-side entry.** **No exit from the top/bottom of a node.**

```svg
<!-- Downward (destination lane > source lane) -->
<path d="M {rx},{src_cy} H {dst_cx - 8} Q {dst_cx},{src_cy} {dst_cx},{src_cy + 8} V {dst_top}"
      fill="none" stroke="…" stroke-width="…" marker-end="…"/>

<!-- Upward (destination lane < source lane) -->
<path d="M {rx},{src_cy} H {dst_cx - 8} Q {dst_cx},{src_cy} {dst_cx},{src_cy - 8} V {dst_bottom}"
      fill="none" stroke="…" stroke-width="…" marker-end="…"/>

<!-- Same lane (adjacent step) -->
<line x1="{src_right}" y1="{lane_cy}" x2="{dst_left}" y2="{lane_cy}"
      stroke="…" stroke-width="…" marker-end="…"/>
```

- **Z-order:** all `<path>` and `<line>` connectors emitted **before** any node `<rect>`.
- **Markers:** exactly one `marker-end` per path. Never `marker-start`.
- **Labels:** all process arrows are unlabelled by default. The step number + the actor lane carry the semantic; a label on every arrow is noise. Only label an arrow if it represents a non-step concept (re-test loop, escalation) — then use a paper-masked rect behind 6.5-px mono text.

### 3.2 Crossings

Avoid. The corridor x position (8 px before destination node) is the only routing column — if two arrows would cross there, **swap step assignments** or **split into two diagrams** rather than introducing a bend-around. Crossings hide the underlying control flow.

---

## 4. Component color override

Any node, lane, or step accepts an optional `color: "#hex"`. Mirrors `type-data-flow.md` §4 and `type-high-level.md` §3.4 so the rule reads identically across types.

### 4.1 Per-node `color`

Applied to:

| Element | Light | Dark |
|---|---|---|
| Container fill (`rect`) | `rgba(C, 0.06)` | `rgba(C_light, 0.10)` |
| Container stroke | `rgba(C, 0.35)` (stroke-width 1) | `rgba(C_light, 0.45)` |
| Role chip fill | `rgba(C, 0.18)` | `rgba(C_light, 0.22)` |
| Role chip text | `C` | `C_light` |
| Title text | `C` | `C_light` |
| Sub-label (in → out) | **unchanged** (muted) | **unchanged** (muted) |
| Tool label | **unchanged** (soft) | **unchanged** (soft) |
| Data-type chips | **unchanged** | **unchanged** |
| Arrows touching this node | **unchanged** — topology-driven | **unchanged** |

`C_light` = the same hex lightened ~15% for dark-mode contrast (e.g., `#b85450` → `#d97a78`).

### 4.2 Per-step `color`

Replaces the step header chip's fill with `rgba(C, 0.20)` and the chip's number text fill with `C`. The legend's matching step entry uses the same colors.

### 4.3 Per-lane `color`

Replaces the lane stripe tint with `rgba(C, 0.04)` and the lane label text fill with `C`. Use sparingly — lane tints are easy to over-apply.

### 4.4 Rules

- **Never on focal nodes / focal steps.** The accent already carries that signal. A `color` on a focal element is ignored.
- **Never on arrows.** Arrows are topology-driven. If you want a colored edge, pick a different `style` from §3, not a color override.
- **Cap at 3 custom-colored elements** per diagram (nodes + lanes + steps combined), in addition to the focal pair (focal node + focal step header). Above 3 the visual signal starts to fragment.
- **Subtitle and tool labels stay muted** regardless of any component `color`.

### 4.5 Semantic palette (recommended)

Same palette as `type-high-level.md`, `type-dp-integration.md`, `type-data-flow.md`:

- `#b85450` rust-red — Security / Identity / Governance (access control, training, approvals)
- `#5a7d9a` slate-blue — Observability / Quality (data quality gates, validation, monitoring)
- `#7a8c47` olive-green — Data Products / Publication (consumer-ready outputs, releases)
- `#8c6d3f` warm-brown — Backup / DR / Archive

---

## 5. Focal rule

The process diagram has three focal slots, exactly one entry each:

- **One focal step** (`steps[j].focal: true`) — typically the analytical or decision pivot (Test, Approve, Validate). Header chip and legend chip render in accent.
- **One focal node** (`nodes[i].focal: true`) — the node that *receives* the critical handoff. Accent border + accent role chip + ink title (title text stays ink so it's still readable; only the border + role chip carry the accent).
- **One focal arrow set** (`style: focal-in` and `focal-out`) — edges into and out of the focal node. Accent solid strokes.

If zero or >1 of any focal slot are declared, halt and ask the user.

---

## 6. Dark mode

| Token | Light | Dark |
|---|---|---|
| Paper | `#f5f5f5` | `#2d3142` |
| Ink | `#2d3142` | `#f5f5f5` |
| Muted | `#4f5d75` | `#bfc0c0` |
| Soft | `#7a8399` | `#8e98ac` |
| Accent | `#eb6c36` | `#f08a59` |
| Dot pattern | `rgba(45,49,66,0.10)` | `rgba(245,245,245,0.10)` |
| Lane tint | `rgba(45,49,66,0.018)` | `rgba(245,245,245,0.025)` |
| Dividers | `rgba(45,49,66,0.12)` | `rgba(245,245,245,0.12)` |
| Label col divider | `rgba(45,49,66,0.20)` | `rgba(245,245,245,0.22)` |
| Default chip fill | `rgba(45,49,66,0.12)` | `rgba(245,245,245,0.12)` |
| Focal chip fill | `rgba(235,108,54,0.20)` | `rgba(240,138,89,0.22)` |
| Default node fill | white | `rgba(245,245,245,0.04)` |
| Default node stroke | `rgba(45,49,66,0.25)` | `rgba(245,245,245,0.20)` |
| Focal node fill | `rgba(235,108,54,0.08)` | `rgba(240,138,89,0.12)` |
| Focal node stroke | `#eb6c36` | `#f08a59` |
| Custom component colors | `C` | `C_light` (lighten ~15%) |

---



---

The dark-mode, reproducibility, data-chip, legend, complexity, anti-pattern, and
worked-example sections are in
[type-process-details.md](type-process-details.md).
