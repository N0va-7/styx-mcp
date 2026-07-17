## Why

OpenSpec was treated as optional (medium+ only), so fixes and tests could drift
from a single contract. Make OpenSpec the default discipline for all work.

## What Changes

- New capability `dev-workflow`: change-required process, specs as contract,
  tests map to scenarios, validate before commit.
- AGENTS.md and openspec/config.yaml rewritten to match.
- No changes to topology / socks-proxy / mcp-async-tasks / transport requirements.

## Capabilities

### New Capabilities

- `dev-workflow`: How contributors and agents MUST use OpenSpec for feat/fix/test

### Modified Capabilities

(none)

## Non-goals

- CI enforcement of openspec validate (optional follow-up)
- Expanding domain specs in this change

## Impact

- Every code/test change needs openspec/changes/<name>/ (slim OK)
- New main spec after archive: openspec/specs/dev-workflow/spec.md
