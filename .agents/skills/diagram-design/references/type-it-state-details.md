# IT current-state details

## 7. Reproducibility checklist (taste gate)

Before emitting SVG, verify **every** item:

1. Eyebrow + title + subtitle present at canonical y-positions (24 / 36 / 52); body padding 32 px.
2. 2..4 zones; each has its uppercase label at the top-left of its zone box, on a paper-masked break in the border.
3. Every component has `id`, `name`. `sub`, `icon`, `kind`, `color` are optional.
4. ≤ 2 components with `kind: focal`; focal styling auto-applied (accent fill 7 %, accent stroke 1.4, italic line-1 sub).
5. Every connector exits the right (or bottom) of source and enters the top (or left) of destination; rounded right-angle Q-bezier `r=8` at every bend; marker triangle visibly touches the destination rectangle edge.
6. Connector labels sit at the **start** of the connector (not mid-segment) and are offset **perpendicular** to the line (5 px gap above for horizontal segments, 6 px gap to the right for vertical segments) — never overlapping the stroke. Paper-fill mask kept behind text; icon (when `icon:` is set) sits left of text inside the same mask.
7. ≤ 3 custom-colored components; none on focal.
8. ≤ 3 footer bars; each spans `viewBox_w − 2*left_pad`; no connectors emerge from any footer.
9. Legend at bottom: hairline separator + one swatch per style actually used.
10. `arrow-label` for connector labels, `eyebrow` for the page eyebrow and zone labels, `title` for the page title, `node-name` for the subtitle and component names, and `sublabel` for technical sub-labels.
11. Markers `#arrow` / `#arrow-link` / `#arrow-accent` defined once in `<defs>`; no inline marker definitions.
12. Dark variant: resolve every semantic token through its dark-mode value; custom colors are lightened ~15 %.

---

## 8. Anti-patterns

- **Diagonal arrows.** The NatStat replication has one (analyst → LegacyPortal). The new type forbids it — always rounded right-angle Q-bezier.
- **Marker not touching the target.** Path ends at the centroid or stops short of the border.
- **Inline `text` connector labels without a mask rect.** The connector line can bleed through the text and it becomes unreadable.
- **Labels sitting on top of the connector line, mid-segment.** Labels belong at the *start* of the connector with a perpendicular margin (see §3.5) — burying them in the middle of the line hides the source-to-destination direction and forces the reader's eye to fight the mask.
- **Tiny text badges as icons.** The source uses 7-px `DB` / `APP` / `EXT` badges; this type uses real 24-px catalog icons. Text badges are only acceptable as the label text, not as the component "icon."
- **Custom color on a focal component.** Focal always wins; user-set `color` silently ignored on `kind: focal`.
- **Footer bar wired to one component.** Footer = cross-cutting layer-wide concern; a connector from a footer to a specific component is a category error (use `type-dp-integration.md`'s AUTH-line pattern only when the footer service truly authenticates *all* components, and even then the line lands at the zone bottom edge, not at a specific tool).
- **> 16 total components or > 5 per zone.** Density cap; split into two diagrams.
- **Mixing orientations within one diagram.** Pick one — `horizontal` or `vertical` — and apply it to every zone.
- **Using `kind: focal` to flag every painful thing.** Focal exists for ≤ 2 narrative pain-points; for "this is bad but not headline-bad", use `color: "#b85450"` rust-red instead.

---

## 9. Examples

- `assets/example-it-state.html` — minimal light (NatStat canonical: 3 zones, 9 components, 8 connectors, 0 footer bars, SQL Server tinted olive). Gallery default.
- `assets/example-it-state-dark.html` — same, dark skin.
- `assets/example-it-state-full.html` — same, editorial-card frame with summary cards.
- Extended color-override and footer-bar variants are not bundled with this installed skill; use the canonical examples above as the starting point for a project-specific variant.

---

## 10. Worked YAML — full inputs for `example-it-state.html`

The complete inputs that map to the shipped canonical example. Every coordinate in that SVG is derivable from §2 applied to these inputs.

```yaml
title:    "Current IT Landscape"
subtitle: "Data pipeline before the platform"
eyebrow:  "NatStat · Before the platform"

orientation: horizontal

zones:
  - name: "COLLECTION"
    components:
      - { id: survey-solutions, name: "Survey Solutions", sub: "CAPI · PostgreSQL",          icon: postgres }
      - { id: aspnet,           name: "ASP.NET Apps",     sub: "migration · admin portals", icon: server   }
      - { id: civil-reg,        name: "Civil Registry",   sub: "external · CRVS data",      icon: database, kind: external }
  - name: "PROCESSING"
    components:
      - { id: shared-drive,  name: "Shared Drive",     sub: "No version control · Windows file share", icon: file,      kind: focal }
      - { id: analyst-mach,  name: "Analyst Machines", sub: "SPSS · SAS · Stata · Excel",              icon: desktop }
      - { id: sql-server,    name: "SQL Server",       sub: "on-premises · core RDBMS",                icon: sqlserver, color: "#7a8c47" }
  - name: "DISSEMINATION"
    components:
      - { id: legacy-portal,   name: "LegacyPortal",      sub: "manual bottleneck",     icon: cloud,    kind: focal }
      - { id: natstat-website, name: "NatStat Website",   sub: "public · static pages", icon: internet }
      - { id: ministry,        name: "Ministry Partners", sub: "~6 ministries",         icon: users,    kind: external }

connectors:
  - { from: survey-solutions, to: shared-drive,   label: "CSV",    icon: csv,   style: link }
  - { from: aspnet,           to: shared-drive,   label: "EMAIL",  icon: file,  style: link }
  - { from: civil-reg,        to: shared-drive,   label: "EXCEL",  icon: excel, style: link, dashed: true }
  - { from: shared-drive,     to: analyst-mach,   label: "COPY",                style: accent, dashed: true }
  - { from: analyst-mach,     to: sql-server,     label: "LOAD",                style: neutral }
  - { from: analyst-mach,     to: legacy-portal,   label: "EXCEL",  icon: excel, style: accent }
  - { from: legacy-portal,    to: natstat-website, label: "WEB",                 style: neutral }
  - { from: natstat-website,  to: ministry,        label: "CSV DL", icon: csv,   style: link, dashed: true }

dark: false
```

### 10.1 What this YAML proves

- `n_zones = 3`, components per zone `= [3, 3, 3]`, custom color count = 1 (SQL Server), focal count = 2 (Shared Drive, LegacyPortal), external count = 2 (Civil Registry, Ministry Partners).
- Zone widths in canonical: 256 / 360 / 272 ⇒ `viewBox_w = 16 + 256 + 20 + 360 + 20 + 272 + 16 = 960` ✓
- `viewBox_h = 52 + 360 + 40 + 24 = 500` (no footer bars) ✓
- Shared Drive (focal) at zone 2, row 0: `x = 340, y = 80, w = 264, h = 68` (focal stretches to 68 to fit 2-line sub) ✓
- LegacyPortal (focal) at zone 3, row 0: `x = 704, y = 80, w = 208, h = 60` ✓
- SQL Server (custom olive) at zone 2, row 2: container fill `rgba(122,140,71,0.06)`, stroke `rgba(122,140,71,0.45)`, name text `#7a8c47` ✓
- Connectors 4, 5 (within zone 2) and 7, 8 (within zone 3) are simple vertical `<line>` elements. Cross-zone connectors take rule-compliant routes (see SKILL.md §6 rules 4 & 5):
  - **All three Survey-side → Shared Drive connectors (C1 / C2 / C3) enter Shared Drive's LEFT edge.** A top-edge entry would push the marker body (7 px back along travel, given `refX = 7`) *inside* the destination box, where the box's paper-fill mask hides it — only a 1-pixel tip would peek above the stroke. Entering the left edge with a right-going path keeps the body outside the box and the arrow visible (~7 px shown to the left of the box edge). The three left-edge attach points are fanned at **y = 108 / 124 / 140** (16-px spacing, well above the 12 px rule-4 minimum).
  - **C1** (Survey → Shared Drive) source y matches landing y: single horizontal `M 252,108 H 340`. No bends needed.
  - **C2** (ASP.NET → Shared Drive) detours up through zone-2 background — vertical at `x = 316` (clear of Shared Drive's left edge at `x = 340`): `H 308 Q 316,196 316,188 V 132 Q 316,124 324,124 H 340`. Lands at `(340, 124)`.
  - **C3** (Civil Registry → Shared Drive) detours up through zone-2 background — vertical at `x = 332` (clear of Analyst Machines, which starts at `x = 340`): `H 324 Q 332,284 332,276 V 148 Q 332,140 340,140`. Lands at `(340, 140)` via a final Q-bend (no trailing H needed).
  - **C6** (Analyst Machines → LegacyPortal) cannot use a direct H+Q+V into LegacyPortal's left edge — Analyst Machines and LegacyPortal are in different rows, and the direct horizontal at `y = 268` would cross NatStat Website. It detours through the zone gap and **over** LegacyPortal: `H 654 Q 662,268 662,260 V 72 Q 662,64 670,64 H 800 Q 808,64 808,72 V 80` — vertical at `x = 662` (in zone gap), horizontal at `y = 64` (above LegacyPortal top), then down into LegacyPortal's top center. The path enters LegacyPortal from **above** going down, so the arrow body lives above the box (visible) and only the 1-px tip enters the box.

**Marker-visibility rule of thumb:** with the standard arrow marker (`markerWidth = 8`, `refX = 7`), the arrow body extends 7 px *backwards* along the path direction from the endpoint. For the arrow to remain visible, that 7-px tail must sit *outside* the destination box. Translation:
- Entering a **TOP edge going UP** (path direction up, box below) → body inside box, **only 1 px visible. Avoid this.**
- Entering a **TOP edge going DOWN** (path direction down, box below) → body above box, ~7 px visible. ✓
- Entering a **LEFT edge going RIGHT** → body to the left of box, ~7 px visible. ✓
- Entering a **RIGHT edge going LEFT** → body to the right of box, ~7 px visible. ✓
- Entering a **BOTTOM edge going DOWN** (box above) → body inside box, **only 1 px visible. Avoid this.**
- Entering a **BOTTOM edge going UP** (box above) → body below box, ~7 px visible. ✓

When the source row matches the destination row's y range (e.g., Survey at y=108 with Shared Drive at y=80–148), prefer **side-edge** entry — a single horizontal path with a fully visible arrow. When the source row is offset, detour through the destination's nearest zone background to enter a side edge rather than approaching a top/bottom edge from the wrong side.

The extended example (§9 line 4) demonstrates footer bars + a third custom color and proves `viewBox_h` grows correctly when `N_footer > 0`.
