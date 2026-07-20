## Why

Public GitHub surface undersells the product (About text, topics) and has
small doc/CI gaps that hurt trust: Go badge vs go.mod, Cursor-only MCP setup,
missing CI, and a TBD intranet-scan Purpose.

## What Changes

- README / README_ZH: Go badge, reconnect feature line, Codex MCP setup section
- Fix `intranet-scan` spec Purpose (no behavior change)
- Add minimal GitHub Actions CI (`go test`, `openspec validate` when available)
- Update GitHub repository description and topics (via `gh`)

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — docs/meta only; no requirement behavior change)

## Non-goals
- Product protocol changes
- Pushing lab exploit scripts
- Full release binary pipeline

## Impact
- Public docs and CI only
