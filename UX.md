# Vakt — UX & Design System

**Status:** Locked for MVP · **Stack:** React + Tailwind + shadcn/ui (Radix primitives) · **Reference points:** Vercel, Linear

This document fixes the visual and interaction language before any component is written. It is the source of truth for tokens; components derive from it and never hard-code a value.

---

## 1. The vibe

**Vakt should feel like infrastructure, not like a to-do app.**

It is a quiet household system that runs for months and mostly wants to be left alone. The user opens it to check that something is handled, taps once, and leaves. Everything in the design should serve that: fast, calm, legible at a glance, and completely free of encouragement. No streaks, no confetti, no progress rings, no motivational copy. The dog either got fed or it didn't.

This is why Vercel and Linear are the right reference points. Both earn their character through restraint — near-monochrome surfaces, one accent used sparingly, thin borders instead of shadows, typography doing the work that decoration does elsewhere. Both are also dense without being cluttered, which matters because the feed shows a whole household's obligations on a phone screen.

Three principles that resolve most arguments:

**Borders, not shadows.** Separation comes from a 1px line. Shadows are reserved exclusively for things that float above the page — popovers, dialogs, dropdowns.

**The accent is a scarce resource.** Violet appears on the primary action, the focus ring, and active navigation. Nowhere else. A screen where three things are violet has no primary action.

**State is communicated by color *and* form.** Never color alone — a chip has a label, an overdue item has a position in the feed. Color is reinforcement, not the signal.

---

## 2. Color

### 2.1 Why violet

The five task states claim green, amber, gray, blue, and red. An accent in any of those hues creates a false relationship — a teal primary button beside an `active` green chip reads as connected when it isn't. Violet is the only hue not already carrying meaning, and it sits naturally in the Linear family.

Neutrals carry a slight violet bias (hue 252) rather than being pure gray. A pure `#808080` reads as unconsidered; a neutral that leans imperceptibly toward the accent reads as chosen.

### 2.2 Tokens

shadcn/ui convention — HSL triplets, no `hsl()` wrapper, consumed as `hsl(var(--background))`.

**Vakt ships one theme: dark.** There is no light palette and no toggle. `prefers-color-scheme` is not consulted.

```css
@layer base {
  :root {
    --background:          252 10% 6%;
    --foreground:          252  8% 93%;

    --card:                252 10% 9%;
    --card-foreground:     252  8% 93%;

    --popover:             252 10% 10%;
    --popover-foreground:  252  8% 93%;

    --primary:             252 62% 68%;
    --primary-foreground:  252 20% 10%;

    --secondary:           252  8% 14%;
    --secondary-foreground:252  8% 93%;

    --muted:               252  8% 14%;
    --muted-foreground:    252  6% 60%;

    --accent:              252  8% 16%;
    --accent-foreground:   252  8% 93%;

    --destructive:           0 62% 52%;
    --destructive-foreground:0 0% 100%;

    --border:              252  8% 17%;
    --input:               252  8% 17%;
    --ring:                252 62% 68%;

    --radius: 0.5rem;
  }
}
```

**Keep `darkMode: ["class"]` in the Tailwind config and ship `<html class="dark">` permanently.** Tokens on `:root` are what components actually consume, but copied shadcn components occasionally carry a `dark:` utility internally — without the class those silently resolve to their light branch. The class costs nothing and removes a whole category of subtle bug.

Note `--accent` in shadcn's vocabulary means *hover surface*, not brand colour. Vakt's brand accent is `--primary`. This is a common source of confusion — don't repurpose it.

The background is not pure black. `6%` lightness keeps surfaces from vibrating against OLED black and lets `--card` at `9%` read as a distinct layer.

`--primary-foreground` is near-black because `--primary` is a *light* violet — dark text on a light button. Inverting this is the most likely token mistake.

### 2.3 State colors

Product-specific, outside shadcn's token set. Each state needs a foreground and a background.

```css
:root {
  --state-active:        142 46% 62%;   --state-active-bg:        142 34% 13%;
  --state-triggered:      32 78% 62%;   --state-triggered-bg:      32 50% 14%;
  --state-paused:        252  6% 62%;   --state-paused-bg:        252  8% 16%;
  --state-completed:     214 62% 68%;   --state-completed-bg:     214 44% 15%;
  --state-failed:          0 70% 68%;   --state-failed-bg:          0 46% 15%;
}
```

| State | Reads as | Note |
|---|---|---|
| `active` | Green | Scheduled and healthy |
| `triggered` | Amber | Dispatched, awaiting fulfillment. **Not** "delivered" — see SDD §3 |
| `paused` | Gray | Deliberately suspended; must look inert, not broken |
| `completed` | Blue | Finished one-time task |
| `failed` | Red | Needs a human. The only state that should draw the eye across a full screen |

`failed` is the sole state permitted to break the calm. Everything else recedes.

### 2.4 Rules

- Never fill a large surface with a state color — chips and 2px rails only.
- `--destructive` is for destructive *actions* (delete file). `--state-failed` is for task condition. They are different concepts that happen to both be red.
- Every color resolves through a token. A hex literal in a component is a bug.
- **Token discipline matters more, not less, in a single-theme app.** Dark-only removes the pressure that normally catches hard-coded colours — a literal that happens to look right on a dark ground will never be caught by a theme switch. Holding the rule is also what keeps a light theme cheap if it is ever wanted: a second token block, not a component audit.

---

## 3. Typography

**Geist Sans** for interface, **Geist Mono** for directives, IDs, timestamps, and file paths. Geist is Vercel's family, which puts the brief's reference point directly in the product, and Geist Mono is genuinely well-suited to the thing Vakt is *about* — `@schedule(0 8 * * *)` should look like what it is.

```css
--font-sans: "Geist", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
--font-mono: "Geist Mono", ui-monospace, SFMono-Regular, Menlo, monospace;
```

### Scale

| Role | Size / line-height | Weight | Tracking |
|---|---|---|---|
| Page title | 24 / 1.2 | 600 | -0.02em |
| Section heading | 16 / 1.4 | 600 | -0.01em |
| Task title | 15 / 1.4 | 500 | -0.005em |
| Body | 14 / 1.55 | 400 | 0 |
| Secondary / meta | 13 / 1.45 | 400 | 0 |
| Chip / label | 12 / 1.2 | 500 | 0.01em |
| Mono inline | 13 / 1.4 | 400 | 0 |

Negative tracking on larger sizes is what makes type read as Vercel/Linear rather than default Tailwind. It matters more than any other single typographic choice here.

**Uppercase** is reserved for small section eyebrows, with `0.08em` tracking. Never on buttons, never on chips.

**Tabular numerals** (`font-variant-numeric: tabular-nums`) wherever times or counts align in a column — the feed's next-fire column above all.

---

## 4. Space, radius, elevation

**Spacing** is Tailwind's 4px scale, unmodified. Card padding `16px` mobile / `20px` desktop. Feed row gap `8px`. Section gap `32px`.

**Radius** derives from `--radius: 0.5rem`:

| Element | Radius |
|---|---|
| Card, dialog, popover | 8px (`--radius`) |
| Button, input, select | 6px (`calc(var(--radius) - 2px)`) |
| Chip, badge | 4px (`calc(var(--radius) - 4px)`) |
| Avatar, status dot | full |

Don't apply `rounded-lg` uniformly. Differentiated radii are part of the hierarchy.

**Elevation** — four levels, and three of them have no shadow:

| Level | Treatment | Used by |
|---|---|---|
| Base | `--background` | Page |
| Raised | `--card` + 1px `--border` | Cards, list rows |
| Overlay | `--popover` + border + `shadow-md` | Dropdown, popover, tooltip |
| Modal | `--popover` + border + `shadow-lg` + scrim | Dialog, sheet |

Scrim is `hsl(var(--background) / 0.8)` with `backdrop-blur-sm`.

---

## 5. Motion

Linear's speed is a large part of why it feels good. Slow is the enemy.

| Interaction | Duration | Easing |
|---|---|---|
| Hover, focus, color | 120ms | `ease-out` |
| Chip / small transform | 160ms | `cubic-bezier(0.16, 1, 0.3, 1)` |
| Popover, dropdown enter | 180ms | `cubic-bezier(0.16, 1, 0.3, 1)` |
| Dialog, sheet enter | 220ms | `cubic-bezier(0.16, 1, 0.3, 1)` |
| Any exit | 120ms | `ease-in` |

Nothing exceeds 220ms. Exits are always faster than entrances.

**No decorative animation.** No skeleton shimmer (use a static muted block), no pulsing, no spinners longer than a moment. Optimistic updates mean most actions have no loading state at all — the chip changes instantly and rolls back on failure.

Wrap everything in `@media (prefers-reduced-motion: reduce)` and drop to opacity-only.

---

## 6. Components

shadcn components are copied into `web/src/components/ui/` and may be edited. Product components live in `web/src/components/`.

| Need | Base | Notes |
|---|---|---|
| Task card | `Card` | Custom layout; state rail on the left edge, 2px |
| State chip | `Badge` | Five variants from §2.3 |
| Quick actions | `Button` ghost/sm + `DropdownMenu` | Fulfill inline; pause/skip in overflow |
| Feed grouping | `Separator` + custom | Overdue / Today / Upcoming / Triggered |
| Vault browser | `Collapsible` | Custom tree |
| Toasts | `Sonner` | Action confirmations, rollback notices |
| Push enrolment | `Dialog` | Plain-language iOS install guidance |
| Date/time helper | `Popover` + `Calendar` | `@once`, `@skip_until` |
| Enum helper | `Select` | `@target`, `@state` — options from `/api/v1/directives` |
| Cron helper | `Popover` + `Tabs` | Shortcuts / raw input, with server-rendered next-runs |
| Editor | **CodeMirror 6** | Not shadcn. Themed from our tokens — see §7 |
| Accessory bar | Custom | No shadcn equivalent; see §8 |

**Icons: Lucide**, 16px default, 1.5px stroke. Never emoji in UI chrome.

### Task card anatomy

```
┌─┬─────────────────────────────────────────────┐
│ │  Feed the dog                    in 2h 14m  │   title 15/500, next-fire mono 13 tabular
│ │  @dog_feed · /Household/Routines.md         │   mono 12, muted
│ │                                             │
│ │  [active]              ✓ Fulfill      ⋯     │   chip + ghost actions
└─┴─────────────────────────────────────────────┘
  ↑ 2px state rail
```

The rail carries state color so a scan down the feed reads status without parsing chips.

---

## 7. Editor theming

CodeMirror 6 is styled from the same tokens — it must not look like a foreign element.

| Token | Source |
|---|---|
| Editor background | `--background` |
| Gutter | `--muted-foreground` on `--background` |
| Selection | `--primary` at 18% |
| Cursor | `--primary` |
| Active line | `--muted` at 40% |
| Directive `@name` | `--primary` |
| Directive value | `--foreground` |
| System directive (`@last_triggered`) | `--muted-foreground`, italic |
| Error diagnostic | wavy underline `--destructive` |
| Warning diagnostic | wavy underline `--state-triggered` |

Editor font is Geist Mono at 14/1.6 — larger than inline mono, because this is sustained reading on a phone.

---

## 8. Mobile & PWA

Vakt is a phone app first. It runs in iOS standalone mode from the Home Screen.

- **Touch targets ≥ 44×44px.** Density comes from tight type and thin borders, never from shrinking hit areas.
- **Safe areas.** `env(safe-area-inset-*)` on all fixed chrome. The home indicator will eat a bottom bar that ignores it.
- **`100dvh`, never `100vh`** — iOS Safari's toolbar makes `vh` wrong.
- **Accessory bar** positions against `window.visualViewport`, not the layout viewport. It must never cover the caret.
- **Overscroll.** `overscroll-behavior: contain` on scrollers so pull-to-refresh doesn't fight the feed.
- **No hover-only affordances.** Every hover state needs a touch equivalent.
- **Single dark theme, declared everywhere it can flash.** `<meta name="theme-color">`, and the manifest's `background_color` and `theme_color`, all take `--background`. Set `apple-mobile-web-app-status-bar-style` to `black-translucent`. Without this, a user whose phone is in light mode gets a white splash screen and a white status bar on every launch — the most visible way a dark-only PWA goes wrong.

---

## 9. Anti-patterns

Things that would break the character, listed because each is a plausible instinct:

- Gradients on surfaces or buttons.
- Colored or glowing shadows.
- Emoji as iconography or section markers.
- The accent on more than one element per screen region.
- Large surfaces filled with a state color.
- Skeleton shimmer, spinners, progress rings.
- Uppercase buttons or chips.
- Celebration of any kind on task completion.
- Hard-coded hex values in components.
- `rounded-lg` applied uniformly to everything.

---

## 10. Where this lands in delivery

`create-frontend-scaffold` (M1) installs Tailwind and runs `shadcn init`, writing §2's tokens and §3's fonts at that moment. `shadcn init` generates both a light and a dark block by default — delete the light one and move the dark values onto `:root`, per §2.2. Getting this right before the task runs avoids retrofitting every component later.

`build-pwa-shell` (M1) is where §8's manifest and meta colours land. They are easy to forget and immediately visible when missed.

Component work lands in M4 (`app shell`, `feed`, `task card`, `quick actions`, `vault browser`, `enrolment`) and M5 (`editor`, `helpers`, `accessory bar`). Both consume this document; neither should introduce a token.
