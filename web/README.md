# web

The Vakt PWA — React + TypeScript, built with Vite, styled with Tailwind CSS
and shadcn/ui per `UX.md`. See the repository root `CLAUDE.md`, `SDD.md`
(architecture) and `UX.md` (design system) before changing anything here.

This is a scaffold: `src/App.tsx` is a placeholder that exercises the build
pipeline (Tailwind utilities, a shadcn component, the dark design tokens). No
real screens live here yet — those land in later milestones (M4/M5).

## Scripts

- `npm run dev` — Vite dev server.
- `npm run build` — type-checks (`tsc -b`) then builds the production bundle
  to `dist/` (Vite's default output directory).
- `npm run preview` — serve the production build locally.
- `npm run lint` — [oxlint](https://oxc.rs).
- `npm run format` / `npm run format:check` — Prettier.

## Build output

`npm run build` emits to `web/dist/`. Per `SDD.md` §2.3, the Go binary
(`internal/api`) is meant to serve the API and this built bundle from the
same origin in production. Wiring that static-file serving into the Go
binary is out of scope for this task (`create-frontend-scaffold`) — a later
task should point its handler at `web/dist/`.

## Design system

- Tailwind CSS v4 (`@tailwindcss/vite` plugin, CSS-first config — no
  `tailwind.config.js`).
- shadcn/ui, Radix-based (`components.json`), components copied into
  `src/components/ui/`.
- Design tokens and the dark-only theme live in `src/index.css`, transcribed
  from `UX.md` §2.2/§2.3. There is one theme; `prefers-color-scheme` is not
  consulted, and `<html class="dark">` is set permanently in `index.html`.
- Fonts: Geist (interface) and Geist Mono (directives, IDs, timestamps,
  file paths), self-hosted via `@fontsource/geist` and
  `@fontsource/geist-mono`.
