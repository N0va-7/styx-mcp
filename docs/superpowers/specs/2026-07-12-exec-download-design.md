# Async exec + download_file

**Date:** 2026-07-12  
**Status:** Approved

## Goal

Non-interactive remote command execution and file download for MCP/LLM clients. All operations return `task_id` and complete via `get_task_status` (same as `upload_file`).

## Tools

### `exec`

- Args: `node_id`, `command`, optional `timeout_sec` (default 30, max 120), optional `workdir`
- Returns: `{ task_id }`
- Result: `exit_code`, `stdout`, `stderr`, `truncated`, `timed_out`, `duration_ms`
- No PTY; `sh -c`; combined output cap 512KiB

### `download_file`

- Args: `node_id`, `remote_path`, `local_path`
- Returns: `{ task_id }`
- Result: `local_path`, `bytes`, `sha256`
- Single-slice transfer first (max 32MiB); path may be absolute on agent

## Protocol

Append (end of iota):

- `EXECREQ` / `ExecReq`
- `EXECRES` / `ExecRes`

Reuse `FILEDOWNREQ` / `FILEDOWNRES` / `FILESTATREQ` / `FILEDATA` for download (agent → controller).

## Compatibility

Matching controller/agent required for new tools.
