package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// workspacePath returns ~/.picoclaw/workspace/<file>
func workspacePath(file string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", file)
}

// ensureWorkspaceDirs creates all required workspace directories.
func ensureWorkspaceDirs() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".picoclaw", "workspace"),
		filepath.Join(home, ".picoclaw", "workspace", "skills"),
		filepath.Join(home, ".picoclaw", "workspace", "memory"),
		filepath.Join(home, ".picoclaw", "workspace", "sessions"),
		filepath.Join(home, ".picoclaw", "workspace", "cron"),
		filepath.Join(home, ".picoclaw", "workspace", "bin"),
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}
}

// resolveAgentIdentity extracts agent name and owner name from existing SOUL.md.
// Falls back to safe defaults if SOUL.md is absent or not yet written.
func resolveAgentIdentity() (agentName, ownerName string) {
	agentName = "Claw"
	ownerName = "the owner"

	data, err := os.ReadFile(workspacePath("SOUL.md"))
	if err != nil {
		return
	}
	content := string(data)

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// Parse "# SOUL.md — AgentName" from the first heading
		if strings.HasPrefix(line, "# SOUL.md") {
			parts := strings.SplitN(line, "—", 2)
			if len(parts) == 2 {
				agentName = strings.TrimSpace(parts[1])
			}
		}
		// Parse "the digital twin of OwnerName" line
		if strings.Contains(line, "digital twin of") {
			parts := strings.SplitN(line, "digital twin of", 2)
			if len(parts) == 2 {
				ownerName = strings.TrimSpace(strings.TrimRight(parts[1], "."))
			}
		}
		if agentName != "Claw" && ownerName != "the owner" {
			break
		}
	}
	return
}

// writeWorkspaceFiles generates workspace markdown files.
// tools may be nil when called after the soul step before OAuth connects.
//
// Two categories:
//   - User-owned (IDENTITY, USER, HEARTBEAT, MEMORY): written once, never overwritten.
//   - System-managed (AGENTS, TOOLS): always regenerated so they stay in sync
//     with whatever tools are currently installed or removed from claw-tools.dev.
func writeWorkspaceFiles(tools []ClawTool, agentName, ownerName string) {
	ensureWorkspaceDirs()

	// User-owned — write once, respect any edits the user has made
	writeIfAbsent(workspacePath("IDENTITY.md"), generateIdentityMD(agentName, ownerName))
	writeIfAbsent(workspacePath("USER.md"), generateUserMD(ownerName))
	writeIfAbsent(workspacePath("HEARTBEAT.md"), generateHeartbeatMD())
	writeIfAbsent(workspacePath("MEMORY.md"), "# Memory\n\n<!-- PicoClaw appends learnings here over time -->\n")

	// System-managed — always regenerated to reflect current installed tools
	writeAlways(workspacePath("AGENTS.md"), generateAgentsMD(tools))
	writeAlways(workspacePath("TOOLS.md"), generateToolsMD(tools))
}

// writeIfAbsent writes content to path only if the file does not already exist.
func writeIfAbsent(path, content string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "workspace: %s already exists — skipping\n", filepath.Base(path))
		return
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "workspace: could not write %s: %v\n", filepath.Base(path), err)
		return
	}
	fmt.Fprintf(os.Stderr, "workspace: wrote %s\n", filepath.Base(path))
}

// writeAlways writes content to path every time, overwriting any existing content.
// Used for system-managed files that must stay in sync with installed tools.
func writeAlways(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "workspace: could not write %s: %v\n", filepath.Base(path), err)
		return
	}
	fmt.Fprintf(os.Stderr, "workspace: updated %s\n", filepath.Base(path))
}

// ── File generators ───────────────────────────────────────────────────────────

func generateIdentityMD(agentName, ownerName string) string {
	return fmt.Sprintf(`# IDENTITY.md

## Who I am
I am %s, the personal AI agent of %s.
I am not a generic assistant — I am a specific agent with a specific owner.
I run on a Raspberry Pi, always on, always available via Telegram.

## My purpose
- Manage %s's email, calendar, and daily workflow
- Proactively surface what matters — do not wait to be asked
- Summarise, filter, and prioritise so %s can focus on decisions, not noise

## My boundaries
- I act on behalf of %s only
- I never make irreversible changes without explicit confirmation
- I am honest about what I can and cannot do
- When uncertain, I ask before acting
`, agentName, ownerName, ownerName, ownerName, ownerName)
}

func generateUserMD(ownerName string) string {
	return fmt.Sprintf(`# USER.md — Owner preferences

## Owner
Name: %s
Timezone: auto-detect from system
Language: English
Delivery: Telegram

## Communication preferences
- Keep responses concise — get to the point
- Use bullet points for lists of 3 or more items
- Do not pad responses with filler phrases
- When summarising emails, lead with what needs action

## Email preferences
- Priority: direct emails from real people > replies to threads > newsletters
- Skip: promotional emails, automated notifications, marketing
- Notetaker emails (Otter, Fireflies, Fathom): always include in morning briefing

## Morning briefing preferences
- Deliver at the scheduled time without being asked
- Keep total length under 400 words
- Structure: greeting → calendar → emails → meeting notes → actions
- If nothing important: still send a brief "all clear" message

## What %s does not want
- Raw JSON output
- Overly formal language
- Responses that start with "Certainly!" or "Of course!"
- Unnecessary disclaimers about being an AI
`, ownerName, ownerName)
}

func generateHeartbeatMD() string {
	return `# HEARTBEAT.md — Scheduled tasks

## Morning briefing
Schedule: 0 7 * * *
Description: Daily morning summary delivered to Telegram

When triggered:
1. Fetch unread emails from the last 24 hours (gmail_search: is:unread newer_than:1d)
2. Fetch today's calendar events (gcal_today)
3. Identify notetaker summary emails by subject keywords: "meeting summary", "transcript", "fathom", "otter", "fireflies"
4. Compose a briefing following the structure in AGENTS.md
5. Send to Telegram

If no emails and no calendar events: send "Good morning — inbox clear, nothing on the calendar today."

## Keep-alive
Schedule: */30 * * * *
Description: Lightweight check to confirm agent is responsive

When triggered:
- Do nothing visible — this is a heartbeat to keep the process warm
- Log: "heartbeat ok"
`
}

func generateAgentsMD(tools []ClawTool) string {
	var sb strings.Builder

	sb.WriteString(`# AGENTS.md — Behaviour rules

## Core rules
- Always respond in plain, readable prose — never raw JSON
- When a tool returns data, summarise it for the human
- If a tool call fails, say so clearly and suggest what to do next
- Never claim you cannot access email or calendar — you have local tools for both
- Prefer action over explanation: do the thing, then briefly say what you did

## When to use tools

### Email
Use Gmail tools when the user:
- Asks about email, inbox, messages, or unread items
- Mentions a person and wants to know if they emailed
- Asks for a morning briefing or daily summary
- Says "check my email" or "what did I miss"

Always fetch the last 24 hours unless a different window is specified.
Filter out newsletters, promotions, and automated notifications unless explicitly asked.
Surface: direct emails from real people, replies to threads, emails with action items.

### Calendar
Use Google Calendar tools when the user:
- Asks about their day, schedule, meetings, or appointments
- Asks what is coming up, what is next, or how the week looks
- Is preparing for the morning briefing (always include calendar)

Always include today's events in a morning briefing.
Flag back-to-back meetings, early starts, or late finishes.

### Morning briefing
Trigger automatically via HEARTBEAT.md at the scheduled time.
Can also be triggered on demand: "briefing", "morning summary", "what's on today".

Structure:
1. Good morning greeting with the date
2. Today's calendar — time, title, location or link if present
3. Email summary — unread from real people, grouped by sender or topic
4. Meeting notes — any notetaker summary emails (Otter, Fireflies, Fathom, etc.)
5. One-line action list if anything needs a reply or decision

### Notetaker emails
Otter.ai, Fireflies.ai, Fathom, and similar tools send summary emails after meetings.
Identify them by subject line patterns: "meeting summary", "transcript", "recording", "your fathom".
Extract the meeting title, attendees, and key decisions or action items.
Present them as a clean bullet list — not the raw email body.

## Output format rules
- Use plain text with minimal markdown
- Bullet points for lists of 3 or more items
- Bold only for headings within a message
- Keep the total morning briefing under 400 words
- Time format: 9:00 AM not 09:00:00
`)

	if len(tools) > 0 {
		sb.WriteString("\n## Available MCP tools\n\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- **%s** — %s\n", t.Name, t.Description))
			if len(t.MCPTools) > 0 {
				for _, method := range t.MCPTools {
					sb.WriteString(fmt.Sprintf("  - `%s`\n", method))
				}
			}
		}
	} else {
		sb.WriteString("\n## Available MCP tools\n\nNo tools connected yet. Complete the MCP Tools step in the wizard to enable Gmail and Google Calendar.\n")
	}

	return sb.String()
}

func generateToolsMD(tools []ClawTool) string {
	var sb strings.Builder

	sb.WriteString(`# TOOLS.md — Tool reference

These tools run as local MCP servers on this device.
All OAuth tokens are stored at ~/.picoclaw/tokens/ — nothing is sent to the cloud.

## Critical rule
When a tool returns a result, you MUST parse it and present it as readable text.
Never show raw JSON to the user. Never pass through the jsonrpc envelope.

## Result handling
- Array result → summarise each item in plain English
- Empty array → say "nothing found" or "inbox clear"
- Error field present → report the problem clearly, suggest retry
- Snippet fields contain HTML entities (&amp; &#39; &lt; &gt;) — always decode before showing
- Snippet fields may contain invisible tracking characters (͏) — strip them

`)

	if len(tools) == 0 {
		sb.WriteString("No tools installed yet. Connect a Google account in the wizard to enable Gmail and Google Calendar.\n")
		return sb.String()
	}

	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", t.Name, t.Description))

		switch t.ID {
		case "gmail":
			sb.WriteString(`### Result fields
| Field   | Description |
|---------|-------------|
| id      | Gmail message ID |
| subject | Email subject line |
| from    | Sender name and address |
| date    | Sent date and time |
| snippet | First ~200 chars of body (HTML-encoded, may have tracking chars) |
| account | Which Gmail account this came from |

### How to present emails
Format each email as: **Sender** — Subject (Date)
Group by: action required → replies → FYI → newsletters (skip newsletters unless asked)

`)
		case "gcal":
			sb.WriteString(`### Result fields
| Field       | Description |
|-------------|-------------|
| id          | Calendar event ID |
| summary     | Event title |
| start       | Start time (ISO 8601) |
| end         | End time (ISO 8601) |
| location    | Room, address, or video link |
| description | Event notes |
| attendees   | List of attendees |

### How to present events
Format each event as: **HH:MM AM – HH:MM AM** — Title (Location if present)
Flag: back-to-back meetings (< 5 min gap), events before 8 AM or after 6 PM.

`)
		default:
			sb.WriteString("Refer to the tool's own documentation for field details.\n\n")
		}
	}

	return sb.String()
}
