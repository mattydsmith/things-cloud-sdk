# Web UI Design Spec — "Ting"

> *Ting* (Danish for "things") — a calm, minimal web interface for Things Cloud.

## Design Philosophy

Inspired by Danish/Scandinavian design principles:

- **Hygge** — warm, comfortable, inviting. Not cold or clinical.
- **Funktionalisme** — every element serves a purpose. Nothing decorative.
- **Hvid rum** (white space) — generous breathing room. Let content rest.
- **Materialer** — honest materials. No fake shadows, no skeuomorphism, no gradients.

Think: Hay furniture, Kinfolk magazine, Dieter Rams' "less but better."

---

## Color Palette

Warm neutrals inspired by Danish ceramics and natural materials.

### Light Mode (primary)

| Role | Color | Hex | Usage |
|------|-------|-----|-------|
| Background | Warm white | `#FAF9F7` | Page background |
| Surface | Soft cream | `#F3F1ED` | Cards, sidebar |
| Border | Warm gray | `#E5E1DB` | Subtle dividers |
| Text primary | Charcoal | `#2C2C2C` | Headings, task titles |
| Text secondary | Warm gray | `#8A8580` | Metadata, dates, counts |
| Text tertiary | Light gray | `#B5B0A9` | Placeholders, disabled |
| Accent | Terracotta | `#C4704B` | Active states, current view indicator |
| Accent hover | Deep terracotta | `#A85A38` | Hover/press states |
| Success | Sage green | `#7A9B7A` | Completed tasks |
| Danger | Muted rose | `#C47070` | Destructive actions |

### Dark Mode (future)

| Role | Color | Hex |
|------|-------|-----|
| Background | Deep charcoal | `#1A1918` |
| Surface | Warm dark | `#242220` |
| Border | Dark warm gray | `#3A3735` |
| Text primary | Warm white | `#E8E4DF` |
| Text secondary | Muted tan | `#9A9590` |
| Accent | Warm terracotta | `#D4805B` |

---

## Typography

**Font:** `Inter` (clean, Scandinavian feel, excellent readability)

| Element | Size | Weight | Letter-spacing |
|---------|------|--------|----------------|
| Page title | 20px | 600 (semi-bold) | -0.01em |
| Section heading | 13px | 600 | 0.02em (uppercase) |
| Task title | 15px | 400 | -0.005em |
| Task title (completed) | 15px | 400 | — (+ line-through, text-secondary color) |
| Metadata | 13px | 400 | 0 |
| Small label | 11px | 500 | 0.03em (uppercase) |
| Input text | 15px | 400 | -0.005em |

---

## Spacing System

Base unit: `4px`. All spacing is multiples of 4.

| Token | Value | Usage |
|-------|-------|-------|
| `xs` | 4px | Inline gaps, icon padding |
| `sm` | 8px | Between related elements |
| `md` | 16px | Component internal padding |
| `lg` | 24px | Between sections |
| `xl` | 32px | Page margins |
| `2xl` | 48px | Major section gaps |

---

## Layout

### Structure

```
┌─────────────────────────────────────────────────┐
│  Sidebar (240px)  │  Main Content               │
│                   │                              │
│  [Logo/Name]      │  [View Title]     [Actions]  │
│                   │                              │
│  VIEWS            │  [Task list]                 │
│  ○ Inbox     12   │  ┌─────────────────────────┐ │
│  ● Today      5   │  │ ○ Task title            │ │
│  ○ Upcoming   3   │  │   Project · Tomorrow    │ │
│  ○ Anytime   28   │  ├─────────────────────────┤ │
│  ○ Someday    8   │  │ ○ Task title            │ │
│                   │  │   Area · Tag             │ │
│  PROJECTS         │  └─────────────────────────┘ │
│  ○ Project A  4   │                              │
│  ○ Project B  2   │                              │
│                   │                              │
│  AREAS            │         [Detail Panel →]     │
│  ○ Work           │         (slides from right)  │
│  ○ Personal       │                              │
│                   │                              │
│  TAGS             │                              │
│  ○ urgent         │                              │
│  ○ waiting        │                              │
└─────────────────────────────────────────────────┘
```

### Sidebar

- Fixed width: `240px`, collapsible to icon-only `56px`
- Background: Surface color (`#F3F1ED`)
- Active view indicator: terracotta left border (3px), slightly bolder text
- Section headers: uppercase, small label style, `text-tertiary`
- Task counts: right-aligned, `text-secondary`, `13px`
- No icons for views — just text (clean, Scandinavian)
- Hover: subtle background shift

### Main Content

- Max width: none (fills available space)
- Padding: `32px` top, `48px` sides
- View title: top-left, `20px semi-bold`
- Task count badge: next to title, `text-secondary`

### Task Row

- Height: auto, min `48px`
- Padding: `12px 0`
- Separated by 1px border in `border` color
- **Checkbox**: 18px circle, 1.5px stroke, `border` color. Filled with `success` on complete.
- **Title**: `15px`, `text-primary`. Strikethrough + `text-secondary` when complete.
- **Metadata line**: below title, `13px`, `text-secondary`. Shows: project, area, scheduled date, deadline, tags.
- **Hover**: entire row gets subtle background (`#F3F1ED`)
- **No priority colors, no badges, no complexity** — just the task.

### Detail Panel

- Slides in from right, `480px` wide
- Background: `background` color
- Left border: 1px `border` color
- Contains: title (editable), notes (editable), metadata fields, checklist
- Close: click outside or press `Esc`

---

## Components

### Checkbox

```
Unchecked:  ○  (18px circle, 1.5px stroke, #E5E1DB)
Hover:      ○  (stroke becomes #8A8580)
Checked:    ●  (filled #7A9B7A, white checkmark)
```

### Tag Chip

```
┌──────────┐
│ tag name │  Background: #F3F1ED, text: #8A8580
└──────────┘  Border-radius: 4px, padding: 2px 8px, font: 12px
```

### Date Badge

- Overdue: `text-danger` color (`#C47070`)
- Today: `text-accent` color (`#C4704B`)
- Future: `text-secondary` color
- Format: "Tomorrow", "Mon", "Mar 28", "Mar 28, 2027"

### Empty State

- Centered vertically and horizontally
- Single line of text, `text-tertiary`, `15px`
- e.g., "No tasks for today" / "Inbox is empty"
- No illustrations, no icons — just words.

### Login Page

- Centered card on `background`
- App name "Ting" in `20px semi-bold`
- Single password field (the API key)
- Single button: "Sign in" — terracotta background, white text
- Minimal. Nothing else on the page.

---

## Interactions

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `n` | New task |
| `j` / `k` | Navigate up/down |
| `Enter` | Open task detail |
| `Esc` | Close panel / deselect |
| `x` | Complete selected task |
| `⌘ K` | Command palette |
| `1-5` | Switch view (inbox/today/upcoming/anytime/someday) |

### Transitions

- Panel slide-in: `200ms ease-out`
- Checkbox fill: `150ms ease`
- Background hover: `100ms ease`
- No bouncing, no spring physics — **calm motion**.

---

## Responsive Behavior

| Breakpoint | Behavior |
|------------|----------|
| `> 1024px` | Full sidebar + content + detail panel |
| `768–1024px` | Collapsed sidebar (icons only) + content |
| `< 768px` | No sidebar, bottom tab bar, full-screen views |

---

## What This Is Not

- Not a Things clone — different colors, different layout density, different interaction model
- Not Material Design — no elevation system, no FABs, no ripple effects
- Not a "web app" feel — no loading spinners everywhere, no skeleton screens, no toast notifications cluttering the screen
- Calm, quiet, functional. Like a well-made wooden chair.
