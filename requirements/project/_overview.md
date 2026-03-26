# Project Overview

## Team and Roles

A single developer (P1.1) fills all project roles — requirements, design,
implementation, testing, and release. An AI assistant participates in
requirements elicitation and code development. See P1 for details.

## Approach

All technical choices are made by the developer (P2 — no imposed choices).
Technology is Go 1.25+ with the Anthropic Go SDK (P5). Requirements follow the
PEGS Standard Plan with AI-assisted elicitation and review (P7). Development is
iterative with no fixed timeline (P3).

## Timeline

Six milestones ordered by dependency: Basic Conversation (M1) → Tool Use (M2)
→ HITL (M3), Extended Thinking (M4), Deterministic Logic (M5) → Example
Application (M6, started at M2). No hard deadlines — pace is driven by
available time and learning goals. See P3 for details.

## Key Risks

The highest-impact risks are **API design lock-in** (P6.2) — getting the public
API wrong early — and **no external stakeholders** (P6.5) — no feedback loop
beyond the developer's own perspective. The example application (M6) mitigates
both by validating the API through real use. See P6 for the full risk register.

## Chapter Index

| Chapter | Contents |
|---------|----------|
| [p1](p1-roles.md) | Roles and personnel |
| [p2](p2-technical-choices.md) | Imposed technical decisions |
| [p3](p3-schedule.md) | Schedule and milestones |
| [p4](p4-deliverables.md) | Tasks and deliverables |
| [p5](p5-technology.md) | Required technology elements |
| [p6](p6-risks.md) | Risk and mitigation analysis |
| [p7](p7-process.md) | Requirements process and reporting |
