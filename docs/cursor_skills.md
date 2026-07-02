# Cursor Skills Reference

This document provides an overview of all available Cursor Agent Skills, their purposes, and how to use them.

## Overview

Cursor Skills are specialized instructions that teach the AI agent how to perform specific tasks. When a skill is relevant to your request, the agent automatically reads and follows its instructions. You can also explicitly reference skills in your prompts.

---

## Quick Reference Table

| Category | Skill | Primary Use Case | Trigger Keywords |
|----------|-------|-----------------|------------------|
| **Automation** | automate | Create scheduled/event-driven agents | "create automation", "scheduled agent", "when PR opens" |
| **Automation** | loop | Recurring tasks | "/loop", "every 5 minutes", "monitor" |
| **Automation** | babysit | Keep PR merge-ready | "babysit PR", "get PR to merge" |
| **PR & Code** | split-to-prs | Split changes into PRs | "split PR", "break up branch" |
| **PR & Code** | sdk | Programmatic Cursor agents | "@cursor/sdk", "cursor-sdk", "Agent.create" |
| **Agent Config** | create-rule | Persistent AI guidance | "create rule", "coding standards", ".cursor/rules" |
| **Agent Config** | create-skill | New agent capabilities | "create skill", "SKILL.md" |
| **Agent Config** | create-hook | Custom agent event logic | "create hook", "gate commands", "audit actions" |
| **Visualization** | canvas | Rich data visualization | Charts, tables, analyses, MCP data display |
| **Editor & CLI** | update-cursor-settings | Editor preferences | "change font", "theme", "settings.json" |
| **Editor & CLI** | statusline | CLI prompt customization | "status line", "CLI status bar" |

---

## Automation & Scheduling

### Automate

**Purpose:** Create Cursor Automations - scheduled or event-triggered agent workflows.

**When to Use:**
- When you want to create a new Cursor Automation
- Setting up scheduled Cursor agents (e.g., "run this every day at 9am")
- Creating event-driven workflows (GitHub PR events, Slack messages, PagerDuty incidents, etc.)

**Key Features:**
- Supports multiple trigger types: schedule (cron), GitHub/GitLab events, Slack events, Linear events, PagerDuty incidents, Sentry issues, webhooks
- Can configure tools like PR management, Slack posting, MCP server integration
- Opens the Automations editor with a pre-filled draft for final configuration

**Usage Note:** Must be used from the Agents Window in Cursor. The skill guides you through defining trigger, tools, and prompt, then opens the Automations editor for final setup.

---

### Loop

**Purpose:** Run a prompt or skill on a recurring or variable interval.

**When to Use:**
- Running recurring local tasks
- Monitoring deployments or CI status
- Periodic code checks or reports

**Syntax:**
- `/loop 5m /foo` - Run `/foo` every 5 minutes
- `/loop 30s check status` - Check status every 30 seconds
- `/loop check deploy every 5m` - Alternative syntax
- `/loop <prompt>` (no interval) - Dynamic mode, agent chooses timing

**Key Features:**
- Fixed schedule with configurable intervals
- Dynamic schedule that adapts based on events
- Can watch for specific events (git changes, log patterns, file modifications)

---

### Babysit

**Purpose:** Keep a PR merge-ready by continuously monitoring and resolving issues.

**When to Use:**
- When you want to get a PR to a merge-ready state
- Need automated handling of merge conflicts, PR comments, and CI failures

**Key Features:**
- Intelligently resolves merge conflicts while preserving intent
- Reviews and addresses unresolved PR comments (including Bugbot)
- Fixes CI issues caused by changes within the PR's scope
- Continues monitoring until the PR is mergeable, green, and all comments are triaged

**Example:** "Babysit this PR until it's ready to merge"

---

## PR & Code Management

### Split to PRs

**Purpose:** Split a large set of changes into smaller, reviewable PRs.

**When to Use:**
- When you have a large branch with multiple logical changes
- Breaking up work for easier code review
- Organizing changes by ownership or concern

**Key Features:**
- Analyzes current changes vs default branch
- Considers CODEOWNERS for reviewer boundaries
- Creates independent or stacked PRs as appropriate
- Saves recoverable snapshots before moving work

**Process:**
1. Check the current state (committed + uncommitted changes)
2. Propose split with PR titles and scope
3. Wait for user approval
4. Execute the split (create branches, commits, PRs)
5. Report back with PR URLs

---

### SDK

**Purpose:** Guide building apps, scripts, or CI pipelines using the Cursor SDK.

**When to Use:**
- Integrating Cursor agents into scripts or automation
- Running Cursor agents programmatically
- Building CI/CD pipelines with Cursor
- Creating bots or services that use Cursor

**SDKs Available:**
- **TypeScript:** `@cursor/sdk` (npm)
- **Python:** `cursor-sdk` (pip)

**Key Patterns:**

1. **One-shot** (`Agent.prompt`):
```typescript
const result = await Agent.prompt("Refactor src/utils.ts", {
  apiKey: process.env.CURSOR_API_KEY,
  model: { id: "composer-2.5" },
  local: { cwd: process.cwd() },
});
```

2. **Durable with follow-ups** (`Agent.create` + `agent.send`):
```typescript
await using agent = await Agent.create({...});
const run = await agent.send("Find the bug");
await run.wait();
```

3. **Resume existing agent** (`Agent.resume`):
```typescript
await using agent = await Agent.resume(previousAgentId, {...});
```

**Runtimes:**
- **Local:** Runs on caller's machine against `cwd`
- **Cloud:** Runs on Cursor-hosted VM against cloned repo

---

## Agent Configuration & Customization

### Create Rule

**Purpose:** Create persistent AI guidance rules for your project.

**When to Use:**
- Adding coding standards to your project
- Setting up project conventions
- Configuring file-specific patterns
- Creating persistent context for the AI agent

**Key Features:**
- Rules are `.mdc` files in `.cursor/rules/`
- Can apply always (`alwaysApply: true`) or only to specific files (`globs: **/*.ts`)
- Under 50 lines recommended for conciseness

**Example Rule Structure:**
```markdown
---
description: TypeScript coding standards
globs: **/*.ts
alwaysApply: false
---

# Error Handling

[Your rule content here...]
```

---

### Create Skill

**Purpose:** Create new Cursor Agent Skills for specialized workflows.

**When to Use:**
- Authoring a new skill for repeated workflows
- Creating domain-specific agent capabilities
- Documenting specialized processes with executable scripts

**Key Features:**
- Skills are directories containing a `SKILL.md` file
- Can include reference docs, examples, and utility scripts
- Personal skills: `~/.cursor/skills/`
- Project skills: `.cursor/skills/`

**Best Practices:**
- Keep SKILL.md under 500 lines
- Write descriptions in third person
- Include both WHAT the skill does and WHEN to use it
- Use progressive disclosure for detailed content

---

### Create Hook

**Purpose:** Create Cursor hooks that run custom logic before or after agent events.

**When to Use:**
- Automating behavior around agent events
- Creating security gates (blocking dangerous commands)
- Auditing agent actions
- Post-processing file edits (auto-formatting)
- Injecting context into tool calls

**Key Features:**
- **Command hooks:** Scripts that receive JSON on stdin and return JSON on stdout
- **Prompt hooks:** LLM-based policy decisions
- Multiple event types: `sessionStart`, `preToolUse`, `postToolUse`, `beforeShellExecution`, `afterFileEdit`, `subagentStart`, and more

**Locations:**
- Project hooks: `.cursor/hooks.json` and `.cursor/hooks/*`
- User hooks: `~/.cursor/hooks.json` and `~/.cursor/hooks/*`

**Example Events:**
- `beforeShellExecution` - Gate or audit terminal commands
- `afterFileEdit` - Auto-format files after edits
- `preToolUse` - Block or rewrite specific tool calls

---

## Visualization

### Canvas

**Purpose:** Create live React applications that display beside the chat for rich data visualization.

**When to Use:**
- Quantitative analyses and metrics breakdowns
- Billing or account investigations with structured findings
- Security audits or architecture reviews
- Data from MCP tools (Datadog, Databricks, etc.) where data IS the deliverable
- Tables with many rows, charts, timelines, or interactive explorations

**When NOT to Use:**
- When the user asks for work in a specific tool (e.g., "create a Datadog dashboard")
- For targeted debugging or active development
- Short factual answers or one-off file edits

**Key Features:**
- Single `.canvas.tsx` file per canvas
- Uses `cursor/canvas` SDK components
- Supports charts, tables, cards, and custom layouts
- No external dependencies or network calls allowed

---

## Editor & CLI Configuration

### Update Cursor Settings

**Purpose:** Modify Cursor/VSCode user settings in `settings.json`.

**When to Use:**
- Changing editor font size, tab size, or theme
- Enabling format on save, auto save
- Modifying any editor preferences

**Settings Location:**
| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/Cursor/User/settings.json` |
| Linux | `~/.config/Cursor/User/settings.json` |
| Windows | `%APPDATA%\Cursor\User\settings.json` |

**Common Settings:**
| Request | Setting |
|---------|---------|
| Bigger/smaller font | `editor.fontSize` |
| Change tab size | `editor.tabSize` |
| Format on save | `editor.formatOnSave` |
| Word wrap | `editor.wordWrap` |
| Change theme | `workbench.colorTheme` |
| Hide minimap | `editor.minimap.enabled` |
| Auto save | `files.autoSave` |

---

### Status Line

**Purpose:** Configure a custom status line in the CLI that displays session context.

**When to Use:**
- Customizing the CLI prompt footer
- Displaying model info, context usage, or git branch
- Adding session-specific information above the prompt

**Configuration:** Add to `~/.cursor/cli-config.json`:
```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.cursor/statusline.sh",
    "padding": 2
  }
}
```

**Available Data:**
- Model info (id, display name, parameters)
- Context window usage (tokens, percentage)
- Session info (id, name, transcript path)
- Workspace info (current directory, project directory)
- Git info (can be fetched in script)
- Vim mode (when enabled)

**Features:**
- Multi-line output supported
- ANSI color codes supported
- Receives JSON payload on stdin
