# Process details

## 7. Reproducibility checklist (taste gate)

Before emitting SVG, verify **every** item:

1. `viewBox = "0 0 {viewBox_w} {viewBox_h}"` derived via §2.
2. Header strip at `y=0..36`; legend strip at `y=legend_y_top..viewBox_h` (`legend_y_top = 36 + n_lanes * 80`).
3. Every node at `(step_cx(j) - 50, lane_y_top(k) + 8)` size `100×64`.
4. Empty cells render nothing — no placeholder rect, no text.
5. Exactly **one** focal step (`steps[j].focal: true`).
6. Exactly **one** focal node (`nodes[i].focal: true`).
7. Focal-touching arrows use `style: focal-in` / `focal-out` (accent).
8. All other arrows `style: normal` (muted solid) or `style: trigger` (muted dashed). Unlabelled by default.
9. All arrows emitted before any node rect (z-order rule).
10. Single-bend right-angle routing only — exit right, enter top/bottom. No diagonals. Q-bezier `r=8` at each bend.
11. Custom component colors ≤ 3 (in addition to the focal pair). Arrows never recolored by component `color`.
12. Subtitle and tool labels stay muted regardless of any component `color`. Input chip skipped on first step's nodes, output chip skipped on last step's nodes.

---

## 8. Data-type chips reference (input + output)

Same catalog as `type-data-flow.md` §8.

- **Input chip** at `(node_x+4, node_y+54)` — bottom-**left**. Payload entering the node.
- **Output chip** at `(node_x+80, node_y+54)` — bottom-**right**. Payload leaving the node.
- Either chip may be omitted (first/last step, unknown payload).

### Chip codes

| Code | Color (light) | Color (dark) | Meaning |
|------|---------------|--------------|---------|
| `LS` | `#7c8f6f` sage | `#9caf8f` | List / assignment / task |
| `DB` | `#5e7a9b` dusty-blue | `#82a0c0` | Dataset / tabular records |
| `TB` | `#b8915a` mustard | `#d3ad7a` | Table (analysis-ready) |
| `FL` | `#9c6b50` rust-brown | `#b88670` | File / document / report |
| `WB` | `#6e6479` slate | `#8d8298` | Web / press / public release |
| N/A | omit chip entirely | — | Unknown or not applicable |

Text inside chip: white, font-size 5, weight 700, mono.

Data-type chip colors are a **separate semantic axis** from the per-node color override (§4). Chip colors describe *payload format*; node color describes *concern type*. A node can have both an `out: TB` mustard chip and a rust-red border simultaneously.

---

## 9. Legend (3- or 4-row strip)

Each row introduced by a category label at `x = label_col_w + 4` (= 144). The default legend has **3 rows** (`STEPS` / `DATA TYPE` / `FLOW`); when one or more nodes carry a `color` override (§4), add a 4th `CONCERN` row and grow `legend_h` to 100.

- **Row 1 — `STEPS`** at `y = legend_y_top + 16`: repeat the header chips with their labels. Focal step keeps accent fill.
- **Row 2 — `DATA TYPE`** at `y = legend_y_top + 37`: one swatch per chip type actually used in the diagram. Append a small sub-hint in muted mono: `left chip = input · right chip = output`.
- **Row 3 — `CONCERN`** (only when color overrides are present) at `y = legend_y_top + 58`: one mini-rect per custom color used, with its semantic label.
- **Row 4 — `FLOW`** (position depends on whether `CONCERN` row exists): one segment per arrow style actually used, with marker + label.

---

## 10. Complexity budget

| Dimension | Max |
|---|---|
| Lanes (actors) | 6 |
| Steps | 12 |
| Nodes per lane | Nodes = active steps only — empty cells are invisible |
| Labelled arrows | 0 by default (label only for non-step concepts) |
| Data-type chips per node | 2 (input + output) |
| Custom-colored elements (§4) | 3 (in addition to focal node + focal step) |

Above 6 lanes or 12 steps: split into two diagrams (overview + detail).

---

## 11. Anti-patterns

- **Placeholder empty cells** — if an actor doesn't participate in a step, leave the cell empty (no box, no text).
- **Diagonal arrows** — every connector must have exactly one right-angle bend. No direct straight lines between nodes in different lanes.
- **Left/right port entry on a vertical-dominant arrow** — always exit right, enter top or bottom.
- **More than one focal step / focal node** — pick the single most critical operation.
- **Unlabelled lanes** — every swimlane must identify its actor.
- **All arrows the same style** — orchestration triggers must be dashed to distinguish them from data-flow connectors.
- **`color` override on a focal element** — ignored. Accent always wins.
- **Custom-colored arrows** — connectors are topology-driven; `color` on a node never spreads to its edges.
- **Lane tints over-applied** — a tint on every lane reads as decoration, not signal. Apply to ≤1 lane.
- **Data-type chips in a double-line-name node** — skip the chips or shorten the name to one line.
- **More than 12 steps without splitting** — use an overview + detail pair.

---

## 12. Worked example — full YAML for an extended process variant

An extended process diagram is fully described by the following inputs. Every coordinate in the rendered SVG is derivable from this block via §2 + §3 + §4. This is the canonical proof that the parametric contract works end-to-end.

```yaml
# Quarterly survey — end-to-end workflow (extended variant)
# 6 lanes × 11 steps, 1 focal step + 1 focal node + 3 custom-colored nodes

lanes:
  - { name: ["RD&E"],                 key: "RDE" }
  - { name: ["IT"],                   key: "IT"  }
  - { name: ["FIELD", "SERVICES"],    key: "FLD" }
  - { name: ["SURVEY", "SERVICES"],   key: "SVY" }
  - { name: ["HOUSEHOLD", "UNIT"],    key: "HHU" }
  - { name: ["COMMS &", "MARKETING"], key: "CMM" }

steps:
  - { number: "1",  label: "Design"   }
  - { number: "2",  label: "Assign"   }
  - { number: "3",  label: "Collect",  focal: true }    # focal step header chip
  - { number: "4",  label: "Review"   }
  - { number: "5",  label: "Validate" }
  - { number: "6",  label: "Weight"   }
  - { number: "7",  label: "Clean"    }
  - { number: "8",  label: "Tabulate" }
  - { number: "9",  label: "Approve"  }
  - { number: "10", label: "Publish"  }
  - { number: "11", label: "Upload"   }

nodes:
  - { lane: "RDE", step: 0,  title: "Sample Design",     sub: "Census data → Sample",
      tool: "SAS · Survey Solutions",      chips: {in: null, out: "LS"} }                # first step: no input chip
  - { lane: "IT",  step: 1,  title: "Field Assignment",  sub: "Sample → Field tasks",
      tool: "Survey Solutions",            chips: {in: "LS", out: "LS"} }
  - { lane: "FLD", step: 2,  title: "Data Collection",   sub: "→ 10,464 dwellings",
      tool: "Survey Solutions",            chips: {in: "LS", out: "DB"},  focal: true }   # focal node
  - { lane: "SVY", step: 3,  title: "HQ Review",         sub: "Submissions → Approved",
      tool: "Survey Sol. HQ",              chips: {in: "DB", out: "DB"},  color: "#b85450" }   # rust-red · governance
  - { lane: "IT",  step: 4,  title: "Error Checks",      sub: "Approved → Cleaned",
      tool: "SAS · Scripts",               chips: {in: "DB", out: "DB"},  color: "#5a7d9a" }   # slate-blue · data quality
  - { lane: "RDE", step: 5,  title: "Weight Calculation", sub: "Cleaned → Weighted",
      tool: "SAS",                         chips: null }                                  # 2-line title — chips skipped
  - { lane: "HHU", step: 6,  title: "2° Cleaning",       sub: "Weighted → Analysis",
      tool: "SAS · R · SPSS",              chips: {in: "DB", out: "TB"} }
  - { lane: "HHU", step: 7,  title: "Tables + Brief",    sub: "Analysis → Tables",
      tool: "Excel · SAS",                 chips: {in: "TB", out: "FL"} }
  - { lane: "CMM", step: 8,  title: "Stats Review",      sub: "Tables → Approved",
      tool: "Internal review",             chips: {in: "FL", out: "FL"} }
  - { lane: "CMM", step: 9,  title: "Public Release",    sub: "Approved → Public",
      tool: "Press conference",            chips: {in: "FL", out: "WB"}, color: "#7a8c47" }   # olive-green · data products
  - { lane: "IT",  step: 10, title: "Upload NatStat / SDMX", sub: "Results → Published",
      tool: "Web · SDMX API",              chips: null }                                  # 2-line title — chips skipped

arrows:
  - { from: {lane: "RDE", step: 0}, to: {lane: "IT",  step: 1},  style: "normal"    }
  - { from: {lane: "IT",  step: 1}, to: {lane: "FLD", step: 2},  style: "focal-in"  }     # → focal
  - { from: {lane: "FLD", step: 2}, to: {lane: "SVY", step: 3},  style: "focal-out" }     # ← focal
  - { from: {lane: "SVY", step: 3}, to: {lane: "IT",  step: 4},  style: "normal"    }     # upward
  - { from: {lane: "IT",  step: 4}, to: {lane: "RDE", step: 5},  style: "normal"    }     # upward
  - { from: {lane: "RDE", step: 5}, to: {lane: "HHU", step: 6},  style: "normal"    }     # downward, skips 2 lanes
  - { from: {lane: "HHU", step: 6}, to: {lane: "HHU", step: 7},  style: "normal"    }     # same lane
  - { from: {lane: "HHU", step: 7}, to: {lane: "CMM", step: 8},  style: "normal"    }
  - { from: {lane: "CMM", step: 8}, to: {lane: "CMM", step: 9},  style: "normal"    }     # same lane
  - { from: {lane: "CMM", step: 9}, to: {lane: "IT",  step: 10}, style: "normal"    }     # upward, skips 4 lanes

dark: false
```

### 12.1 What this YAML proves

Run §2 of this reference with these inputs:

- `n_lanes = 6`, `n_steps = 11`, `has_color_row = true` (3 nodes carry `color`).
- `viewBox_w = 140 + 11 * 112 + 28 = 1400`. ✓ matches rendered SVG.
- `legend_h = 100`, `viewBox_h = 36 + 6 * 80 + 100 = 616`. ✓
- Lane y_top = [36, 116, 196, 276, 356, 436]; lane mid = [76, 156, 236, 316, 396, 476]. ✓
- Step cx = [198, 310, 422, 534, 646, 758, 870, 982, 1094, 1208, 1320] (the 8-px content-area gutter shifts every value by 8 from `140 + j*112 + 50`). ✓
- Node 4 (HQ Review): step=3, lane="SVY" (k=3) → x = 534-50 = 484, y = 276+8 = 284. ✓
- Node 5 (Error Checks): step=4, lane="IT" (k=1) → x = 646-50 = 596, y = 116+8 = 124. ✓
- Node 10 (Public Release): step=9, lane="CMM" (k=5) → x = 1208-50 = 1158, y = 436+8 = 444. ✓ *(Rendered uses x=1156 — 2-px tolerance from chip-width rounding on the step "10" label.)*

The two coord drifts on the rightmost two nodes (chip width=20 for two-digit step numbers shifts the chip but not the node center math) are an artifact of the existing hand-tuned example, not a formula failure — a fresh generation from this YAML would produce x=1158 and the diagram would be visually indistinguishable from the shipped version.

### 12.2 Adapting this YAML to a different process

To document a different process, change only the value of these inputs:

- **Lanes**: rename `lanes[k].name` to your team names; update each `nodes[i].lane` to match. Up to 6 lanes.
- **Steps**: rename `steps[j].label`, move `focal: true` to the step that defines the diagram's central claim. Up to 12 steps.
- **Nodes**: write one entry per `(lane, step)` cell that has work. Leave cells empty (no entry) to render nothing.
- **Colors**: choose `color: "#hex"` on at most 3 nodes (§4 cap). Stick to the recommended palette unless brand demands otherwise.
- **Arrows**: declare every edge explicitly with `style: normal | focal-in | focal-out | trigger`. The routing rule (§3.1) fills in the geometry.

Everything else — viewBox sizing, chip positions, legend layout, dark-mode token swap — is derivable. The YAML is the **source of truth**; the SVG is one of many possible renderings of it (light/dark/full all derive from the same inputs with different style tokens).

---

## 13. Examples

- `assets/example-process.html` — minimal light (quarterly survey: 11 steps, 6 divisions, data-type chips). Gallery default.
- `assets/example-process-dark.html` — same, dark skin.
- `assets/example-process-full.html` — same, editorial-card frame.
- Extended color-override variants are not bundled with this installed skill; use the canonical examples above as the starting point for a project-specific variant.
