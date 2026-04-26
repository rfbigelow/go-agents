# go-agents

## Purpose

Provide a reusable Go library for building LLM-based agents. The library
captures common agent development patterns — agentic loop management,
tool dispatch, streaming, and progressive capability addition — so that new
agent projects can focus on domain-specific behavior rather than
infrastructure.

## Key Stakeholders

- **Library developer** — sole developer, building the library for personal
  use and learning.
- **Library consumer** — agent application developers (currently the same
  person). Uses the library to build agent applications against the Anthropic
  API.

## System Overview

go-agents provides four core components: an Agent that manages the
agentic loop, a Completer that bridges to the Anthropic Go SDK for LLM
communication, a Tool Registry
for registering and dispatching tools, and managed Conversation State. The Agent is designed for progressive
capability addition — from simple completions to tool use, human-in-the-loop,
extended thinking, and deterministic logic. All operations are instrumented
with distributed traces (via OpenTelemetry) and structured logs (via slog).

## Environment Summary

- **LLM provider:** Anthropic (via the Anthropic Go SDK)
- **Language:** Go 1.25+
- **Dependencies:** Minimal — stdlib + Anthropic Go SDK + OpenTelemetry Trace API
- **License:** MIT
- **Platform:** Agnostic (no cloud/OS coupling)

## Project Status

M1 (Basic Conversation), M2 (Tool Use), and M3 (HITL Example) implemented.
Requirements continue to evolve alongside implementation.

## How to Read These Requirements

This repository follows the PEGS requirements structure (Project, Environment,
Goals, System). Requirements are organized into four books:

| Book | Purpose | Start Here |
|------|---------|------------|
| **Goals** | Why the project exists and what success looks like | [goals/_overview.md](goals/_overview.md) |
| **Environment** | External context, constraints, and assumptions | [environment/_overview.md](environment/_overview.md) |
| **System** | What the system does and how it behaves | [system/_overview.md](system/_overview.md) |
| **Project** | How development is organized and executed | [project/_overview.md](project/_overview.md) |

Each book contains numbered chapter files (e.g., `g1-context-and-objective.md`)
that follow the PEGS Standard Plan. Change request history and rationale
is in `changes/`.

For a quick orientation, read this file and the four `_overview.md` files.
For detail on any dimension, read the relevant chapter files.

## Acknowledgment

The PEGS structure used here derives from Bertrand Meyer's requirements
engineering work, particularly *Handbook of Requirements and Business
Analysis* (Springer, 2022). The application in this repository is a
personal synthesis adapted to a small, single-developer library project
and should not be taken as an endorsed or canonical use of the method.
