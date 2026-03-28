# Ting — Web UI Implementation Plan

## What We're Building

A lightweight web companion for Things 3 called **Ting** (Danish for "things"). It lets you view and manage your Things tasks from any browser — primarily aimed at people who don't have access to the Things app (e.g. on a Windows PC at work).

**It is not a full Things replacement.** It covers the core workflow: view tasks, add tasks, triage to Today, assign to existing projects, tag with existing tags.

## Scope

### In Scope

| Feature | Details |
|---------|---------|
| View all standard views | Inbox, Today, Upcoming, Anytime, Someday |
| Today view toggle | Grouped by project/area or flat list |
| Tag filter pills | Filter tasks by tag within any view |
| View projects | Project detail with headings and tasks |
| View areas | Area detail with project cards and loose tasks |
| Create tasks | Title, notes, assign to project, set Today, assign tags |
| Edit tasks | Title, notes, Today toggle, project, tags, checklist |
| Complete/uncomplete tasks | Checkbox toggle |
| Delete tasks | With confirmation dialog (maps to trash on server) |
| Set Today | Simple on/off toggle (star button) |
| Assign existing tags | Toggle from existing tag list |
| Assign to existing project | Dropdown selector |
| Checklist items | View, add, complete, delete |
| Task detail panel | Slide-out panel from right, works on all views |
| Authentication | Cookie-based session using `AUTH_SECRET` (fallback `API_KEY`) |
| Toggle on/off | `WEB_UI=true` env var to enable |

### Out of Scope

- Creating new projects, areas, or tags
- Recurring tasks
- Deadline/date picker
- Drag to reorder
- Logbook / Trash views
- Mobile-specific layouts
- Dark mode (future)

## Tech Stack

| Component | Choice | Why |
|-----------|--------|-----|
| Frontend framework | React 19 + TypeScript | Broad ecosystem, easy for contributors |
| Build tool | Vite | Fast builds, simple config |
| Styling | Tailwind CSS v4 | Utility-first, matches our design system |
| Routing | React Router | Client-side view navigation |
| Data fetching | TanStack Query | Caching, optimistic updates |
| Hosting | Fly.io (embedded in Go binary) | Single deploy, same-origin, no CORS |

## Architecture

### Single Binary Approach

The frontend SPA is built with Vite, then embedded into the Go server binary using `go:embed`. This means:

- **One container, one deploy, one URL** — no separate frontend hosting
- **No CORS** — frontend and API are same-origin
- **Zero runtime cost when disabled** — `WEB_UI=false` skips route registration
- **Simple for self-hosting** — users run one binary + set env vars

```
┌─────────────────────────────────────────┐
│              Go Server Binary           │
│                                         │
│  ┌──────────┐  ┌──────────────────────┐ │
│  │ REST API │  │ Embedded SPA (go:embed)│ │
│  │ /api/*   │  │ / → index.html       │ │
│  │ /mcp     │  │ /assets/* → static   │ │
│  └──────────┘  └──────────────────────┘ │
│                                         │
│         SQLite (/data/things.db)        │
└─────────────────────────────────────────┘
```

### Environment Variables

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `THINGS_USERNAME` | Yes | — | Things Cloud account email |
| `THINGS_PASSWORD` | Yes | — | Things Cloud account password |
| `AUTH_SECRET` | No | (none) | Password for API auth + web UI login (open if unset) |
| `WEB_UI` | No | `false` | Set to `true` to serve the web UI |
| `PORT` | No | `8080` | HTTP server port |
| `DEBUG` | No | `false` | Verbose logging |

### Authentication Flow

1. User visits `/` → served the SPA
2. SPA checks for valid session cookie → if missing, shows login page
3. User enters `AUTH_SECRET` value → POST to `/api/auth/login`
4. Server validates, returns HTTP-only secure cookie (HMAC-signed, stateless)
5. All subsequent API calls include cookie automatically (same-origin)
6. Browser API calls use the session cookie automatically (same-origin)
7. Optional REST bearer auth can continue to exist for scripts during transition
8. MCP endpoint (`/mcp`) stays unauthenticated for Claude.ai compatibility; protecting it is a separate OAuth project
9. Backwards compatible: the current stable server uses `API_KEY` only for `/api/*`. The web UI can introduce `AUTH_SECRET` for browser login while continuing to accept `API_KEY` as a legacy fallback

## API Gaps to Fill

The existing REST API covers most needs but is missing a few endpoints the web UI requires:

| Endpoint | Method | Purpose | Status |
|----------|--------|---------|--------|
| `/api/auth/login` | POST | Cookie-based login | **New** |
| `/api/auth/logout` | POST | Clear session cookie | **New** |
| `/api/tasks/search` | GET | Search by title/note | **New** (exists in MCP) |
| `/api/tasks/:uuid` | GET | Get single task | **New** (exists in MCP) |
| `/api/tasks/:uuid/checklist` | GET | Get checklist items | **New** (exists in MCP) |
| `/api/tasks/move-to-today` | POST | Set task to Today | **New** (exists in MCP) |
| `/api/tasks/move-to-anytime` | POST | Remove from Today | **New** (exists in MCP) |
| `/api/tasks/uncomplete` | POST | Uncomplete task | **New** (exists in MCP) |
| `/api/areas/:uuid` | GET | Get single area | **New** (exists in MCP) |
| `/api/areas/:uuid/tasks` | GET | Tasks in area | **New** (exists in MCP) |
| `/api/projects/:uuid/tasks` | GET | Tasks in project | **New** (exists in MCP) |
| `/api/tasks/inbox` | GET | Inbox tasks | Exists |
| `/api/tasks/today` | GET | Today tasks | Exists |
| `/api/tasks/anytime` | GET | Anytime tasks | Exists |
| `/api/tasks/someday` | GET | Someday tasks | Exists |
| `/api/tasks/upcoming` | GET | Upcoming tasks | Exists |
| `/api/projects` | GET | All projects | Exists |
| `/api/areas` | GET | All areas | Exists |
| `/api/tags` | GET | All tags | Exists |
| `/api/tasks/create` | POST | Create task | Exists |
| `/api/tasks/edit` | POST | Edit task | Exists |
| `/api/tasks/complete` | POST | Complete task | Exists |
| `/api/tasks/trash` | POST | Trash (delete) task | Exists |

Most "new" endpoints already exist as MCP tools — they just need REST wrappers.

## Implementation Phases

### Phase 1 — Backend Prep

1. Add new REST endpoints (move-to-today, uncomplete, search, single task/area, checklist)
2. Add `/api/auth/login` and `/api/auth/logout` with cookie session
3. Add `WEB_UI` env var gate
4. Add `go:embed` scaffold for serving static files from `web/dist/`
5. Update health check: when `WEB_UI=true`, move from `/` to `/api/health`

### Phase 2 — Frontend Scaffold

1. `npm create vite@latest web -- --template react-ts`
2. Install dependencies: `tailwindcss`, `react-router`, `@tanstack/react-query`
3. Set up Tailwind with our design tokens (colours, fonts, spacing from design spec)
4. Create layout shell: sidebar + main content area
5. Create shared components: TaskItem, Checkbox, TagPill, DetailPanel

### Phase 3 — Read-Only Views

1. Login page
2. Today view (grouped + flat toggle, tag filters)
3. Inbox view (with triage buttons)
4. Upcoming view (grouped by date)
5. Anytime view (grouped by area)
6. Someday view (with triage buttons)
7. Project detail view (headings, tag filters, progress)
8. Area detail view (project cards + loose tasks)
9. Task detail slide-out panel (shared across all views)

### Phase 4 — Write Operations

1. Create task (inline "New task" input)
2. Complete/uncomplete (checkbox click)
3. Edit task (title, notes in detail panel)
4. Today toggle (star button in detail panel)
5. Assign to project (dropdown in detail panel)
6. Assign tags (toggle buttons in detail panel)
7. Add/complete/delete checklist items
8. Delete task (with confirmation dialog)

### Phase 5 — Build & Deploy

1. Add Vite build step to Dockerfile
2. Wire `go:embed` to serve `web/dist/`
3. Test locally: `WEB_UI=true go run ./server/`
4. Deploy to Fly.io (see below)

## Deployment

**Ting** (Danish for "things") is a lightweight web interface for your Things 3 tasks. It lets you view, add, and organise tasks from any web browser — handy when you're on a Windows PC, a Linux machine, or a work computer where you can't install Things.

With Ting you can:
- **View your tasks** across all your Things views — Inbox, Today, Upcoming, Anytime, and Someday
- **Add new tasks** and triage them to Today, a project, or Someday
- **Manage tasks** — edit titles and notes, add checklist items, assign tags, move between views, and delete tasks
- **Browse by project and area** — see your tasks organised the same way you've set them up in Things

The server also connects to **Claude** (or any MCP-compatible AI assistant), so you can manage your tasks through conversation — *"What's on my Things today?"*, *"Add a task to buy milk and put it in my Personal project"*.

The whole setup takes about **10 minutes** and is done entirely in your browser — no dev tools or terminal required. Your Things credentials are stored securely on your own server. **The developer has no access to your Things account or your data at any point.**

The server runs on **Fly.io**, a hosting platform with a free tier. You deploy it once, and it stays in sync with your Things account automatically.

### What you'll need

- A **Things Cloud account** (the email and password you use to sync Things on your Mac/iPhone)
- A **GitHub account** (free) — this is where the code lives
- A **Fly.io account** (free) — this is where your app runs

### Step 1 — Create a GitHub account

If you already have a GitHub account, skip to Step 2.

1. Go to https://github.com/join
2. Enter a username, email, and password
3. Follow the prompts to complete signup
4. Verify your email address (check your inbox)

### Step 2 — Fork the repository

Forking creates your own copy of the code, so Fly.io can build and deploy it for you.

1. Make sure you're signed in to GitHub
2. Go to the things-cloud-sdk repository: https://github.com/mattydsmith/things-cloud-sdk
3. Click the **"Fork"** button in the top-right corner
4. On the next page, leave everything as default and click **"Create fork"**
5. You now have your own copy at `https://github.com/<your-username>/things-cloud-sdk`

### Step 3 — Create a Fly.io account

Fly.io is a hosting platform. The free tier is enough to run Ting.

1. Go to https://fly.io
2. Click **"Sign Up"** (or "Get Started")
3. You can sign up with your GitHub account (easiest) or with email
4. You may need to add a credit card — Fly.io requires one on file but the free tier covers Ting's usage

### Step 4 — Create your app on Fly.io

1. In the Fly.io dashboard, click **"Launch App"**
2. Choose **"Launch from GitHub"**
3. If prompted, authorize Fly.io to access your GitHub account
4. Find and select your forked repository (`things-cloud-sdk`)
5. Pick a **region** close to you (e.g. London, Amsterdam, Washington DC) — this is where your data is stored
6. Fly.io will detect the `fly.toml` and `Dockerfile` in the repo — leave these as-is

### Step 5 — Enter your account details

Before deploying, you need to tell the app how to connect to your Things account. These details are stored encrypted on your Fly.io server — **only your app can see them. The developer and nobody else has access to these values.**

In the Fly.io dashboard for your app, go to **Secrets** and add these values:

| Name | Value | What it does |
|------|-------|-------------|
| `THINGS_USERNAME` | Your Things Cloud email (e.g. `you@email.com`) | The email you use to sign in to Things on your Mac or iPhone. This lets the server sync your tasks. |
| `THINGS_PASSWORD` | Your Things Cloud password | The password for your Things account. Stored encrypted on your server — nobody else can see it. |
| `AUTH_SECRET` | Any password you choose | Signs you into the web UI and can back authenticated browser/API flows. It does not protect Claude.ai's MCP connection. |
| `WEB_UI` | `true` | Enables the Ting web interface so you can access your tasks from a browser. |

> **`AUTH_SECRET` is recommended for the web UI itself.** It gives the browser UI a real sign-in instead of leaving the UI open.
>
> **Claude.ai still connects to `/mcp` without auth.** Protecting remote MCP properly is a separate OAuth project, not part of this web UI rollout.

### Step 6 — Deploy

1. Back in your app's dashboard, click **"Deploy"**
2. Wait 2–3 minutes for the build to complete
3. When it's done, Fly.io will show you your app's URL — something like `https://your-app-name.fly.dev`
4. Open that URL in your browser
5. If you set `AUTH_SECRET`, enter it to sign in; otherwise the web UI opens directly

### Step 7 — Connect Claude to your MCP server

The server also works as an MCP (Model Context Protocol) server, which lets Claude read and manage your Things tasks through conversation. You can use the web UI, Claude, or both.

#### Claude.ai (web)

1. Open [claude.ai](https://claude.ai) and sign in
2. Go to **Settings > Connectors > Add custom connector**
3. Set the URL to: `https://your-app-name.fly.dev/mcp` (replace `your-app-name` with the name Fly.io gave your app)
4. Click **Save**

Now you can ask Claude things like *"What's on my Things today?"* or *"Add a task to buy milk"*.

#### Claude Code (CLI)

If you use Claude Code in the terminal, add this to your MCP config file (`~/.claude/mcp.json`):

```json
{
  "mcpServers": {
    "things": {
      "type": "url",
      "url": "https://your-app-name.fly.dev/mcp"
    }
  }
}
```

Replace `your-app-name` with the name Fly.io gave your app.

### Step 8 — Create a Skill

This step is optional, but it makes a **huge** difference to the experience. Without a skill, Claude can read and write your tasks — but it doesn't know anything about how *you* use Things. It doesn't know your projects, your tags, or your workflow. Every conversation, you'd need to explain your system from scratch.

A **Skill** is an instruction file that gives Claude that context permanently. Once installed, Claude knows your project structure, what your tags mean, and how you like to work. The difference is night and day:

- Without a skill: *"Add a task to buy milk"* → Claude creates it in your inbox with no project or tags
- With a skill: *"Add a task to buy milk"* → Claude files it under your "Personal" project, tags it "errands", and sets it for Today

It only takes a few minutes to create one, and Claude does most of the work for you.

#### Let Claude build it for you

The easiest way to create a skill is to let Claude do it. Since Claude already has access to your Things account through the MCP, it can read your projects, areas, tags, and tasks — then ask you a few questions to fill in the gaps.

**Copy and paste this prompt into Claude** (after completing Step 7):

> I'd like you to help me create a Things skill file. Please:
>
> 1. Look at my Things account — fetch all my areas, projects, and tags (with their UUIDs)
> 2. Look at my current tasks to get a sense of how I use Things
> 3. Based on what you find, describe back to me what you think each area, project, and tag is for — make your best guess
> 4. Ask me to correct anything you got wrong, and ask about:
>    - Any tags whose purpose isn't obvious
>    - How I like to schedule tasks (do I use Today as a hard commitment or a soft target?)
>    - Whether I do morning reviews, weekly reviews, or both
>    - Any filing rules (e.g. "work emails always go to my Work project")
> 5. Once we've gone through everything, generate a complete SKILL.md file I can download

Claude will then walk you through a conversation like this:

> **Claude:** *I've looked at your Things account. Here's what I found:*
>
> *You have 3 areas: **Work** (12 projects), **Personal** (4 projects), and **Health** (2 projects).*
>
> *Your tags are: **urgent**, **waiting**, **errands**, **low-energy**, and **deep-work**.*
>
> *Based on your tasks, it looks like you use "urgent" for same-day items, "waiting" for things blocked on other people, and "errands" for tasks that need you to leave the house. Is that right?*
>
> *A few questions:*
> *- What does "deep-work" mean to you? I see it on tasks in both Work and Personal.*
> *- You have 8 tasks in your inbox, some from 2 weeks ago — do you do regular inbox triage?*
> *- ...*

You answer the questions, Claude refines its understanding, and generates a ready-to-use skill file.

#### Installing the skill

Once Claude generates your `SKILL.md`:

1. **Save the file** — Copy Claude's output into a file called `SKILL.md` on your computer
2. **Package it** — Put the file in a folder and zip it:
   ```
   mkdir my-things-daily
   mv SKILL.md my-things-daily/
   zip -r my-things-daily.skill my-things-daily/
   ```
   On Mac, you can also right-click the folder in Finder → "Compress" → rename the `.zip` to `.skill`
3. **Install it** — In Claude, go to **Settings > Skills** and upload the `.skill` file

#### What a finished skill looks like

For reference, here's the kind of file Claude will generate for you:

```markdown
---
name: my-things-daily
description: |
  Daily task management for Things 3. Handles morning reviews,
  inbox triage, and ad-hoc task work.
---

# My Things Daily Management

## My Areas & Projects
| Area | Project | UUID |
|---|---|---|
| Work | Q2 Planning | `abc123-def456-...` |
| Work | Hiring | `ghi789-jkl012-...` |
| Personal | Learn Danish | `mno345-pqr678-...` |
| — | Malta Holiday | `stu901-vwx234-...` |

## My Tags
| Tag | UUID | When to use |
|---|---|---|
| urgent | `aaa-111-...` | Needs attention today, no exceptions |
| waiting | `bbb-222-...` | Blocked on someone else — check back in a few days |
| errands | `ccc-333-...` | Requires leaving the house |
| deep-work | `ddd-444-...` | Needs 1+ hours of focused time, no meetings |

## Morning Review
1. Show today's tasks grouped by project
2. Check inbox count — if anything is older than 3 days, prompt me to triage
3. Highlight overdue items (scheduled before today but still open)

## How I Schedule
- **Today** = I'm committing to doing this today
- **Anytime** = I could do this whenever, no urgency
- **Someday** = Not now, but I don't want to forget it
- New tasks go to **Inbox** unless I specify otherwise

## Default Filing Rules
- Work-related tasks → Work area, ask me which project
- Personal errands → Personal area, tag "errands"
- If I say "remind me" → Inbox, no project

## Communication Style
- Keep summaries brief — bullet points, not paragraphs
- Don't ask for confirmation on simple creates, just do it
- For morning reviews, lead with what's most important
```

See the full [Skills Guide](../../docs/skills.md) for more patterns (weekly reviews, task capture, etc.).

### You're done!

Your Things tasks are now accessible from any web browser — handy when you're away from your Mac or iPhone. And if you've connected Claude, you can manage your tasks through conversation too.

### Keeping it updated

When a new version is released, you can update your server:

1. Go to your fork on GitHub
2. Click **"Sync fork"** → **"Update branch"** — this pulls the latest changes into your copy
3. In the Fly.io dashboard for your app, click **"Deploy"** to rebuild with the new code

We recommend updating manually rather than using auto-deploy — that way, if a release has a problem, your server isn't affected until you choose to update.

### Migrating from an earlier version

If you already have the MCP server running on Fly.io, here's what changes and what you need to do.

#### What's changing

- **Web UI sign-in** — The browser UI will use a password-style login backed by `AUTH_SECRET` or `API_KEY`.
- **`API_KEY` is being renamed to `AUTH_SECRET`** — It does the same thing, the name is just clearer. Your existing `API_KEY` will continue to work — you don't need to change it right away.
- **New optional feature: web UI** — You can now access your tasks from a browser. Set `WEB_UI=true` to turn it on.

> Important: Claude.ai web custom connectors currently rely on the MCP endpoint staying open. Protecting `/mcp` properly will require OAuth, which is a separate project from the web UI rollout.

#### If you already have `API_KEY` set

Most existing users will have set an `API_KEY` when they first installed the server. If that's you:

1. **Update your fork** — Go to your fork on GitHub, click **"Sync fork"** → **"Update branch"**, and redeploy (or let Auto Deploy handle it).

2. **Keep Claude.ai and Claude Code on the same open MCP URL** — the web UI rollout should not require a bearer token on `/mcp`. Existing Claude connectors should continue using `https://your-app-name.fly.dev/mcp` with no auth fields or headers.

3. **Optionally enable the web UI** — If you want to access your tasks from a browser, go to **Secrets** in the Fly.io dashboard and add `WEB_UI` with the value `true`. You'll sign in to the web UI using the same password.

4. **Optionally rename API_KEY** — If you'd like to use the new name for the web UI login and Claude Code, go to **Secrets** in the Fly.io dashboard, add `AUTH_SECRET` with the same value as your `API_KEY`, then remove `API_KEY`. This is purely cosmetic — both names work.

#### If you never set an `API_KEY`

If you skipped setting a password when you first installed, your server is currently unprotected. For the web UI, you'll still want a password for browser login, but Claude connectors should continue using the same open `/mcp` URL.

1. In the Fly.io dashboard, go to **Secrets**
2. Add `AUTH_SECRET` — choose any password you'll remember
3. Redeploy
4. Use that password for the web UI login and for Claude Code if needed

---

## Fly.io Configuration

The repo ships with a pre-configured `fly.toml`. The Fly.io setup needs minor updates for the web UI:

### Updated Dockerfile

```dockerfile
# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go server
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=1 go build -o things-server ./server/

# Stage 3: Runtime
FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/things-server /usr/local/bin/things-server
RUN mkdir -p /data
EXPOSE 8080
CMD ["things-server"]
```

### Set the new env var

```bash
fly secrets set WEB_UI=true
```

### Deploy

```bash
fly deploy
```

That's it. The web UI will be available at `https://<your-app>.fly.dev/`.

### For self-hosters

```bash
# Clone the repo
git clone <repo-url>
cd things-cloud-sdk

# Set env vars
export THINGS_USERNAME=your@email.com
export THINGS_PASSWORD=yourpassword
export AUTH_SECRET=your-secret-key
export WEB_UI=true

# Build and run
cd web && npm ci && npm run build && cd ..
go build -o things-server ./server/
./things-server
```

Or with Docker:

```bash
docker build -t ting .
docker run -p 8080:8080 \
  -e THINGS_USERNAME=your@email.com \
  -e THINGS_PASSWORD=yourpassword \
  -e AUTH_SECRET=your-secret-key \
  -e WEB_UI=true \
  -v things-data:/data \
  ting
```

## File Structure

```
web/
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.ts
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── api/
│   │   └── client.ts              # Fetch wrapper, handles auth
│   ├── components/
│   │   ├── Sidebar.tsx
│   │   ├── TaskItem.tsx
│   │   ├── Checkbox.tsx
│   │   ├── TagPill.tsx
│   │   ├── DetailPanel.tsx
│   │   ├── TodayToggle.tsx
│   │   ├── TagSelector.tsx
│   │   ├── ProjectSelect.tsx
│   │   ├── ChecklistItem.tsx
│   │   ├── ConfirmDialog.tsx
│   │   └── NewTaskInput.tsx
│   ├── views/
│   │   ├── Login.tsx
│   │   ├── Today.tsx
│   │   ├── Inbox.tsx
│   │   ├── Upcoming.tsx
│   │   ├── Anytime.tsx
│   │   ├── Someday.tsx
│   │   ├── Project.tsx
│   │   └── Area.tsx
│   └── styles/
│       └── tokens.css             # CSS variables from design spec
```

## Design Reference

See [`./2026-03-28-web-ui-design-spec.md`](./2026-03-28-web-ui-design-spec.md) for the full design system (colours, typography, spacing). The HTML mockups in `mockups/` are the visual reference for each view.
