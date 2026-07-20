## Why

Public users must clone and `make build` to get binaries. Tag-triggered GitHub
Releases with cross-compiled controller/agent removes that friction.

## What Changes

- Add `.github/workflows/release.yml` on `v*` tags: test → `make build-all` →
  per-platform archives + checksums → GitHub Release
- Brief README note on downloading releases

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `dev-workflow`: document release pipeline expectation (docs)

## Non-goals
- GoReleaser migration
- Extra GOOS/GOARCH beyond existing `make build-all`
- Signing / notarization

## Impact
- CI/CD only; no protocol or runtime behavior change
