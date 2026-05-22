# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository houses the `everything-claude-code` (ECC) project, a comprehensive Claude Code plugin designed to optimize AI agent harnesses. It provides a modular and extensible framework for AI-assisted development across multiple platforms (Claude Code, Cursor, Codex, OpenCode).

The core architecture is organized into:

- `agents/`: Specialized subagents for task delegation and execution.
- `skills/`: Workflow definitions and domain knowledge across various domains and languages.
- `commands/`: User-invocable slash commands for workflow automation.
- `hooks/`: Event-driven automations for session lifecycle, tool usage, and code quality.
- `rules/`: Standardized development guidelines (common and language-specific).
- `scripts/`: Cross-platform utilities.
- `tests/`: Project test suites.
- `.claude-plugin/`: Plugin manifest files.

Additionally, a separate VS Code extension project, "Augment Pro," is located at `tmp\\augpro-extracted\\extension\\`, focusing on AI account management and switching.

## Subagent-First Architecture

To minimize token overhead in the main conversation, Fink utilizes **Subagents with Scoped MCPs**.

- **Global Reasoning**: Only `sequential-thinking` is active globally.
- **Specialized Agents**: Use `@name` to delegate tasks:
    - `@web-researcher`: Deep web search and documentation analysis via DuckDuckGo.
    - `@code-analyst`: Architectural mapping and cross-file relationship indexing via AiDex.
    - `@memory-keeper`: Long-term project-level memory management.

## Expert Reasoning Blueprint (IMMUTABLE)

Fink demands high-density architecture-first reasoning. 

1. **Thinking-Block Primacy**: All deliberation MUST happen in the internal thinking block. The chat output must be strictly Zero-Fluff (Zero Narration). 
2. **OODA Standard**: Every tool call is an **Experiment**. Observe the actual output -> Orient against the codebase -> Decide the next surgical step -> Act.
3. **Architectural Primacy**: Use `aidex_summary` and `aidex_signature` to map logic via AST before reading or editing file implementations.

## Usage

```bash
bunx fink-claude-code-installer
# or
npx fink-claude-code-installer
```

## Known Issues

- The tool utilizes base Node built-ins to guarantee zero-dependency execution.
- If using powershell, the terminal must be restarted after running the tool.
