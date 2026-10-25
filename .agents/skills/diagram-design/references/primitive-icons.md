# Icons (primitive)

A monochrome 24×24 icon library for IT and cloud diagrams. Each icon uses
`currentColor` so it inherits ink from its parent SVG and adapts to the
editorial skin or any user-onboarded brand palette.

## Usage

Find an icon by name in the catalog headings. Copy the fenced `<svg>` snippet
into your diagram. The default size is 24×24. Wrap it in
`<g transform="translate(x,y) scale(s)">` to position and resize it. Set
`color`, `fill`, or `stroke` on the parent group or SVG to control color.

## Core catalog

The common compute, people, and network icons are below. The extended catalog is
split so each reference stays small enough to load on demand.


Find the icon by name (the `### name` headings below). Copy the fenced `<svg>` snippet into your diagram. Default size is 24×24; wrap in `<g transform="translate(x,y) scale(s)">` to position and resize. Set `color`, `fill`, or `stroke` on the parent group/SVG to control color.

Generic icons are stroked (1.5px, hairline, like the rest of the skill); brand silhouettes are filled. Don't mix the two styles in the same diagram unnecessarily.

## Compute

### laptop
User laptop or workstation.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 19l18 0" /> <path d="M5 7a1 1 0 0 1 1 -1h12a1 1 0 0 1 1 1v8a1 1 0 0 1 -1 1h-12a1 1 0 0 1 -1 -1l0 -8" /></svg>
```

Source: Tabler Icons / `device-laptop` (MIT)

### phone
Mobile phone or tablet client.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6 5a2 2 0 0 1 2 -2h8a2 2 0 0 1 2 2v14a2 2 0 0 1 -2 2h-8a2 2 0 0 1 -2 -2v-14" /> <path d="M11 4h2" /> <path d="M12 17v.01" /></svg>
```

Source: Tabler Icons / `device-mobile` (MIT)

### desktop
Desktop computer.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 5a1 1 0 0 1 1 -1h16a1 1 0 0 1 1 1v10a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1v-10" /> <path d="M7 20h10" /> <path d="M9 16v4" /> <path d="M15 16v4" /></svg>
```

Source: Tabler Icons / `device-desktop` (MIT)

### server
Physical server or VM host.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a3 3 0 0 1 3 -3h12a3 3 0 0 1 3 3v2a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3" /> <path d="M3 15a3 3 0 0 1 3 -3h12a3 3 0 0 1 3 3v2a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3l0 -2" /> <path d="M7 8l0 .01" /> <path d="M7 16l0 .01" /></svg>
```

Source: Tabler Icons / `server` (MIT)

### container
Container image or running instance.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l8 4.5l0 9l-8 4.5l-8 -4.5l0 -9l8 -4.5" /> <path d="M12 12l8 -4.5" /> <path d="M12 12l0 9" /> <path d="M12 12l-8 -4.5" /> <path d="M16 5.25l-8 4.5" /></svg>
```

Source: Tabler Icons / `package` (MIT)

### vm
Virtual machine.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16.008v-8.018a1.98 1.98 0 0 0 -1 -1.717l-7 -4.008a2.016 2.016 0 0 0 -2 0l-7 4.008c-.619 .355 -1 1.01 -1 1.718v8.018c0 .709 .381 1.363 1 1.717l7 4.008a2.016 2.016 0 0 0 2 0l7 -4.008c.619 -.355 1 -1.01 1 -1.718" /> <path d="M12 22v-10" /> <path d="M12 12l8.73 -5.04" /> <path d="M3.27 6.96l8.73 5.04" /></svg>
```

Source: Tabler Icons / `cube` (MIT)

## People

### user
End user or single actor.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 7a4 4 0 1 0 8 0a4 4 0 0 0 -8 0" /> <path d="M6 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2" /></svg>
```

Source: Tabler Icons / `user` (MIT)

### users
Group / cohort / team.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5 7a4 4 0 1 0 8 0a4 4 0 1 0 -8 0" /> <path d="M3 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2" /> <path d="M16 3.13a4 4 0 0 1 0 7.75" /> <path d="M21 21v-2a4 4 0 0 0 -3 -3.85" /></svg>
```

Source: Tabler Icons / `users` (MIT)

### admin
Privileged user / admin.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6 21v-2a4 4 0 0 1 4 -4h2" /> <path d="M22 16c0 4 -2.5 6 -3.5 6s-3.5 -2 -3.5 -6c1 0 2.5 -.5 3.5 -1.5c1 1 2.5 1.5 3.5 1.5" /> <path d="M8 7a4 4 0 1 0 8 0a4 4 0 0 0 -8 0" /></svg>
```

Source: Tabler Icons / `user-shield` (MIT)

### robot
Bot, agent, or automated process.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6 6a2 2 0 0 1 2 -2h8a2 2 0 0 1 2 2v4a2 2 0 0 1 -2 2h-8a2 2 0 0 1 -2 -2l0 -4" /> <path d="M12 2v2" /> <path d="M9 12v9" /> <path d="M15 12v9" /> <path d="M5 16l4 -2" /> <path d="M15 14l4 2" /> <path d="M9 18h6" /> <path d="M10 8v.01" /> <path d="M14 8v.01" /></svg>
```

Source: Tabler Icons / `robot` (MIT)

## Network

### cloud
Cloud provider or boundary.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6.657 18c-2.572 0 -4.657 -2.007 -4.657 -4.483c0 -2.475 2.085 -4.482 4.657 -4.482c.393 -1.762 1.794 -3.2 3.675 -3.773c1.88 -.572 3.956 -.193 5.444 1c1.488 1.19 2.162 3.007 1.77 4.769h.99c1.913 0 3.464 1.56 3.464 3.486c0 1.927 -1.551 3.487 -3.465 3.487h-11.878" /></svg>
```

Source: Tabler Icons / `cloud` (MIT)

### internet
Public internet.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0" /> <path d="M3.6 9h16.8" /> <path d="M3.6 15h16.8" /> <path d="M11.5 3a17 17 0 0 0 0 18" /> <path d="M12.5 3a17 17 0 0 1 0 18" /></svg>
```

Source: Tabler Icons / `world` (MIT)

### cdn
CDN or edge cache.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M19.5 7a9 9 0 0 0 -7.5 -4a8.991 8.991 0 0 0 -7.484 4" /> <path d="M11.5 3a16.989 16.989 0 0 0 -1.826 4" /> <path d="M12.5 3a16.989 16.989 0 0 1 1.828 4" /> <path d="M19.5 17a9 9 0 0 1 -7.5 4a8.991 8.991 0 0 1 -7.484 -4" /> <path d="M11.5 21a16.989 16.989 0 0 1 -1.826 -4" /> <path d="M12.5 21a16.989 16.989 0 0 0 1.828 -4" /> <path d="M2 10l1 4l1.5 -4l1.5 4l1 -4" /> <path d="M17 10l1 4l1.5 -4l1.5 4l1 -4" /> <path d="M9.5 10l1 4l1.5 -4l1.5 4l1 -4" /></svg>
```

Source: Tabler Icons / `world-www` (MIT)

### firewall
Firewall or perimeter control.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2l0 -12" /> <path d="M4 8h16" /> <path d="M20 12h-16" /> <path d="M4 16h16" /> <path d="M9 4v4" /> <path d="M14 8v4" /> <path d="M8 12v4" /> <path d="M16 12v4" /> <path d="M11 16v4" /></svg>
```

Source: Tabler Icons / `wall` (MIT)

### vpn
VPN or encrypted tunnel.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a12 12 0 0 0 8.5 3a12 12 0 0 1 -8.5 15a12 12 0 0 1 -8.5 -15a12 12 0 0 0 8.5 -3" /> <path d="M11 11a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M12 12l0 2.5" /></svg>
```

Source: Tabler Icons / `shield-lock` (MIT)

### load-balancer
Load balancer / traffic split.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 17h-8l-3.5 -5h-6.5" /> <path d="M21 7h-8l-3.495 5" /> <path d="M18 10l3 -3l-3 -3" /> <path d="M18 20l3 -3l-3 -3" /></svg>
```

Source: Tabler Icons / `arrows-split` (MIT)

### gateway
API gateway or ingress door.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 12v.01" /> <path d="M3 21h18" /> <path d="M5 21v-16a2 2 0 0 1 2 -2h6m4 10.5v7.5" /> <path d="M21 7h-7m3 -3l-3 3l3 3" /></svg>
```

Source: Tabler Icons / `door-enter` (MIT)

### dns
DNS / name resolution.

```svg
<svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6.5 7.5a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M3 6v5.172a2 2 0 0 0 .586 1.414l7.71 7.71a2.41 2.41 0 0 0 3.408 0l5.592 -5.592a2.41 2.41 0 0 0 0 -3.408l-7.71 -7.71a2 2 0 0 0 -1.414 -.586h-5.172a3 3 0 0 0 -3 3" /></svg>
```

Source: Tabler Icons / `tag` (MIT)



## Extended catalog

- [Data, Kubernetes, action, and DevOps icons](primitive-icons-data-platform.md)
- [Brand and data-stack icons](primitive-icons-brand-data-stack.md)
- [Language, statistical, and file-format icons](primitive-icons-language.md)
