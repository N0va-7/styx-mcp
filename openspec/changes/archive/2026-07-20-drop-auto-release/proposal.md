## Why
GitHub Actions cannot start jobs for this repo (`startup_failure` / BuildFailed).
Auto tag→Release is unreliable; prefer manual `gh release create` for now.

## What Changes
- Remove `.github/workflows/release.yml`
- README notes: manual releases only
- Drop the "tagged releases publish cross-built binaries" requirement

## Capabilities
### Modified Capabilities
- `dev-workflow`: remove auto-release requirement

## Non-goals
- Removing `ci.yml` test workflow
- Deleting existing GitHub Release assets (v0.4.0 stays)
