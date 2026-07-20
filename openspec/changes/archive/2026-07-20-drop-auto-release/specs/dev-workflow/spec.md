## REMOVED Requirements

### Requirement: Tagged releases publish cross-built binaries
Pushing a version tag matching `v*` SHALL trigger CI that builds controller and
agent for the matrix defined by `make build-all` and attaches archives to a
GitHub Release for that tag.

#### Scenario: Tag creates release assets
- **WHEN** a maintainer pushes a git tag such as `v0.3.1`
- **THEN** a GitHub Release for that tag includes platform archives for the
  platforms built by `make build-all` (at least linux-amd64, windows-amd64,
  darwin-arm64) with controller and agent binaries inside
