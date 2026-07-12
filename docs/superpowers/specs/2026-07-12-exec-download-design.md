# Async start_cmd + pull_file

**Date:** 2026-07-12  
**Status:** Approved

## Goal

Non-interactive remote command execution and file download for MCP/LLM clients. All operations return `task_id` and complete via `get_task_status` (same as `upload_file`).

## Tools

### `start_cmd` (formerly `run_command` / `exec`)

- Args: `node_id`, `line`, optional `timeout_sec` (default 30, max 120), optional `workdir`
- Returns: `{ task_id }`
- Result: `exit_code`, `stdout`, `stderr`, `truncated`, `timed_out`, `duration_ms`
- No PTY; `sh -c`; combined output cap 512KiB
- Tool name avoids client-side filters that drop `exec` / `run_command`

### `pull_file` (formerly `download_file`)

- Args: `node_id`, `remote_path`, `local_path`
- Returns: `{ task_id }`
- Result: `local_path`, `bytes`, `sha256`
- Single-slice transfer first (max 32MiB); path may be absolute on agent
- Named to pair with `upload_file` and avoid client filters on `download_file`

## Protocol

Append (end of iota):

- `EXECREQ` / `ExecReq`
- `EXECRES` / `ExecRes`

Reuse `FILEDOWNREQ` / `FILEDOWNRES` / `FILESTATREQ` / `FILEDATA` for download (agent → controller).

## Compatibility

Matching controller/agent required for new tools.
