# Ayati Code vs Pi Evaluation

This directory contains isolated, paired real-model evaluations of Ayati Code
and Pi using the same Fireworks model and the same user prompts.

## Frozen comparison

- Provider: `fireworks`
- Model: `accounts/fireworks/models/deepseek-v4-flash-0731`
- Ayati tools: `shell`
- Pi tools: `read`, `write`, `edit`, `bash`
- Trials are run from empty, agent-specific working directories.
- Pi extensions, skills, prompt templates, themes, and context files are disabled
  so the comparison targets the core coding-agent architecture.
- Real provider calls and normal tool execution are used; no mock model is used.

## Layout

- `prompts/`: exact prompts sent to both agents
- `ayati/`: Ayati work products
- `pi/`: Pi work products
- `evidence/transcripts/`: terminal and session evidence
- `evidence/metrics/`: timing, tool, filesystem, and test results
- `evidence/screenshots/`: rendered website captures
- `evidence/config/`: frozen version and configuration metadata

