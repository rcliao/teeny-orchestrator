# teeny-orchestrator

A lightweight autonomous agent runtime in Go. Loops an LLM with external CLI tools until the task is done.

## What it does

1. Builds context from bootstrap files + session history + learnings
2. Calls an LLM (Anthropic Claude, OpenAI, or any OpenAI-compatible API) with tool definitions
3. Executes tools (external CLI binaries) when the LLM requests them
4. Loops until the LLM stops calling tools or hits max iterations
5. Saves session state for continuity

The key differentiator: an **eval-grounded self-improvement loop**. The orchestrator captures every LLM call to [token-eval](https://github.com/rcliao/token-eval), periodically analyzes patterns via `heartbeat`, stores learnings in [agent-memory](https://github.com/rcliao/agent-memory), and auto-injects those learnings into future context.

## Install

```bash
go install github.com/rcliao/teeny-orchestrator/cmd/teeny-orchestrator@latest
```

Requires Go 1.21+.

## Quick start

```bash
# Initialize config and workspace
teeny-orchestrator init

# Run a one-shot task
teeny-orchestrator run "Summarize the files in this directory"

# Interactive chat
teeny-orchestrator chat

# Start the daemon with scheduled jobs
teeny-orchestrator daemon
```

## Configuration

Config lives at `~/.teeny-claw/config.json`:

```json
{
  "workspace": "~/.teeny-claw/workspace",
  "provider": {
    "name": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "api_key_env": "ANTHROPIC_API_KEY",
    "base_url": ""
  },
  "tools": {
    "path": ["~/.teeny-claw/tools"],
    "timeout": 30
  },
  "session": {
    "dir": "~/.teeny-claw/sessions"
  }
}
```

### Supported Providers

| Provider | `name` | `api_key_env` | Notes |
|----------|--------|---------------|-------|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | Default. Claude models. |
| OpenAI | `openai` | `OPENAI_API_KEY` | GPT-4o, o1, etc. |

Any OpenAI-compatible API works — set `name: "openai"` and configure `base_url` for custom endpoints:

```json
{
  "provider": {
    "name": "openai",
    "model": "llama3",
    "api_key_env": "OLLAMA_API_KEY",
    "base_url": "http://localhost:11434/v1"
  }
}
```

Set your API key: `export ANTHROPIC_API_KEY=sk-...`

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create config directory and default config |
| `run <prompt>` | One-shot: run prompt, print result, exit |
| `chat` | Interactive REPL with session persistence |
| `daemon` | Run scheduled jobs from daemon config |
| `job list` | List configured daemon jobs |
| `job run <name>` | Trigger a specific job immediately |
| `heartbeat` | Run one self-review cycle (analyze call patterns, store learnings) |

### Common flags

- `--session` / `-s` — Session name (default: generates one)
- `--verbose` / `-v` — Verbose logging
- `--max-iterations` — Max tool-call loops (default: 20)
- `--system` — Override system prompt
- `--config` — Config file path

## Tools

Tools are external CLI binaries discovered via `tool.json` manifests:

```json
{
  "name": "agent-memory",
  "description": "Persistent structured storage for agent data",
  "commands": [
    {
      "name": "store",
      "description": "Store a memory entry",
      "parameters": {
        "type": "object",
        "properties": {
          "content": { "type": "string", "description": "Content to store" },
          "tags": { "type": "string", "description": "Comma-separated tags" }
        },
        "required": ["content"]
      }
    }
  ]
}
```

Place manifests in any directory listed in `tools.path`. The orchestrator builds OpenAI-compatible tool schemas from these manifests.

## Daemon & scheduling

Configure scheduled jobs in `~/.teeny-claw/daemon.json`:

```json
{
  "jobs": [
    {
      "name": "daily-review",
      "schedule": "0 9 * * 1-5",
      "prompt": "Review yesterday's work and plan today",
      "session": "daily",
      "enabled": true
    },
    {
      "name": "check-in",
      "schedule": "@every 4h",
      "prompt": "Check project status",
      "session": "checkin",
      "enabled": true
    }
  ]
}
```

**Schedule formats:**
- `@every <duration>` — interval (e.g., `@every 30m`, `@every 2h`)
- `* * * * *` — standard 5-field cron (minute hour day-of-month month day-of-week)

## Architecture

```
cmd/teeny-orchestrator/    CLI entry point (Cobra)
pkg/
  context/     Context builder — assembles system prompt + history + learnings
  provider/    LLM provider adapters (Anthropic, OpenAI-compatible)
  loop/        Core orchestration loop — LLM ↔ tools
  session/     Session persistence (JSON files)
  toolreg/     Tool registry — discovers and executes CLI tools
  scheduler/   Job scheduler — interval + cron expressions
  eval/        Eval client — queries token-eval + agent-memory for self-review
```

## The self-improvement loop

```
heartbeat command
    → queries token-eval for recent LLM calls
    → queries agent-memory for existing learnings
    → LLM analyzes patterns and effectiveness
    → stores new learnings in agent-memory
    → learnings auto-injected into future context
```

This creates a feedback loop: the orchestrator gets better at using tools and structuring prompts over time, grounded in actual execution data rather than vibes.

## Part of teeny-claw

This is one tool in the [teeny-claw](https://github.com/rcliao) constellation:

- **[agent-memory](https://github.com/rcliao/agent-memory)** — Persistent structured storage
- **[token-eval](https://github.com/rcliao/token-eval)** — LLM call capture for eval loops
- **[todo-mgmt](https://github.com/rcliao/todo-mgmt)** — Session-scoped focus tool
- **teeny-orchestrator** — This tool. Ties them all together.

## License

MIT
