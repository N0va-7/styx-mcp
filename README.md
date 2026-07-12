# styx-mcp

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![MCP](https://img.shields.io/badge/MCP-stdio-black)](https://modelcontextprotocol.io/)
[![GitHub stars](https://img.shields.io/github/stars/N0va-7/styx-mcp?style=social)](https://github.com/N0va-7/styx-mcp/stargazers)

**English** | [简体中文](README_ZH.md)

> Multi-hop proxy **controlled by MCP tools** — give Cursor / Claude a Stowaway-style jump network without a separate admin TUI.

```text
LLM (Cursor)  --MCP/stdio-->  controller  <-->  agent  <-->  agent …
                                   |                 |
                              SOCKS / backward    exit traffic
```

## Disclaimer

> **Authorized use only.** Use only on systems you own or have explicit written permission to test (labs, CTF / exam ranges, RoE-covered engagements). Unauthorized use is illegal. You are solely responsible for how you use this software.

**Please read this README (especially [SOCKS vs forward vs backward](#socks-vs-forward-vs-backward)) before use.**

## Why styx-mcp?

| | [Stowaway](https://github.com/ph4ntonn/Stowaway) | **styx-mcp** |
| :--- | :--- | :--- |
| Control plane | Interactive admin TUI | **MCP tools** (LLM / Cursor native) |
| SOCKS listen | Admin side | **Controller** (same idea) |
| Primary user | Human operator | Agent + human |
| Remote shell / download | Yes | Not yet |

Inspired by Stowaway’s multi-hop model; re-oriented for **Model Context Protocol** clients.

## Features

- Tree topology: active (`-c`) / passive (`-l`), multi-hop pivots
- Mutual **HMAC** preauth + optional **TLS** / WebSocket transport
- **SOCKS5** on the controller — local tools exit via a chosen node
- Per-stream **byte-window flow control** (no silent SOCKS drops; matching controller/agent required)
- **Forward** (listen on agent) & **backward** (listen on controller)
- **Async `start_cmd`** (non-interactive `sh -c`, returns `task_id`)
- **Async `pull_file`** / `upload_file` (task + local path; not interactive shell)
- **Upload** files to nodes (path traversal sanitized)
- Async tasks + `get_task_status`
- Cross-compile: Linux / Windows / macOS (`make build-all`)

<details>
<summary><strong>Not included yet</strong></summary>

- Interactive remote shell
- Download from node → controller
- SOCKS username/password
- Full Stowaway-style admin UI

</details>

## Table of contents

- [Quick start](#quick-start)
- [Cursor MCP setup](#cursor-mcp-setup)
- [MCP tools](#mcp-tools)
- [Examples](#examples)
- [CLI flags](#cli-flags)
- [Security notes](#security-notes)
- [Project layout](#project-layout)
- [Acknowledgments](#acknowledgments)
- [License](#license)

## Quick start

```bash
git clone https://github.com/N0va-7/styx-mcp.git
cd styx-mcp
make build          # → release/<os>-<arch>/
```

```bash
# Terminal A — controller (keeps stdio for MCP; for CLI smoke only)
./release/$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')/controller \
  -s change-me -l 127.0.0.1:19137

# Terminal B — agent
./release/.../agent -s change-me -c 127.0.0.1:19137
```

Prefer **Cursor**? Skip terminal A and use the [wrapper](#cursor-mcp-setup) below, then only start the agent.

```bash
make build-all   # linux-amd64 / windows-amd64 / darwin-arm64
make test
```

## Cursor MCP setup

1. `make build` so `release/<os>-<arch>/controller` exists.
2. `~/.cursor/mcp.json` (or project `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "styx-mcp": {
      "command": "/absolute/path/to/styx-mcp/scripts/styx-mcp-wrapper.sh",
      "env": {
        "STYX_SECRET": "change-me",
        "STYX_LISTEN": "0.0.0.0:19137",
        "STYX_LOG": "/tmp/styx-mcp-controller.log"
      }
    }
  }
}
```

3. Cursor → **Settings → MCP** → enable / refresh **styx-mcp**.
4. On the foothold:

```bash
./agent -s change-me -c <controller-ip>:19137
```

| Env | Default | Meaning |
| :--- | :--- | :--- |
| `STYX_SECRET` | `secret` | Shared secret (`-s`) |
| `STYX_LISTEN` | `0.0.0.0:19137` | Agent listen addr on controller |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | Controller log |
| `STYX_BIN_DIR` | `release/<os>-<arch>` | Binary directory override |

Never commit real secrets into public configs.

## MCP tools

| Tool | What it does | Listen / act where |
| :--- | :--- | :--- |
| `list_nodes` | Topology | — |
| `get_node_detail` | Detail | — |
| `add_node_memo` / `delete_node_memo` | Memos | — |
| `start_listener` | Wait for child agents | **Agent** |
| `connect_node` | Dial a child | **Agent** |
| `start_socks` | SOCKS5 for local tools | **Controller** → exit via node |
| `start_forward` | Port forward | **Agent** listen → target |
| `start_backward` | Reverse forward | **Controller** → via node → target |
| `upload_file` | Upload | Controller → agent |
| `pull_file` | Pull file to controller | Agent → controller path |
| `start_cmd` | Non-interactive one-shot | Agent `sh -c` (async `task_id`) |
| `get_task_status` | Poll async work | — |
| `shutdown_node` | Kill node | — |

Long-running calls return `task_id` → poll with `get_task_status`.

### SOCKS vs forward vs backward

| You want… | Use |
| :--- | :--- |
| `curl` / scanners on the **controller host** into an internal net | `start_socks` |
| One **controller** port → one internal `ip:port` | `start_backward` |
| A port **on the foothold** that dials elsewhere | `start_forward` |

`start_forward` is **not** a drop-in for local SOCKS.

## Examples

<details open>
<summary><strong>SOCKS5</strong></summary>

```json
{ "name": "start_socks", "arguments": { "node_id": 0, "address": "127.0.0.1:10801" } }
```

```bash
curl --socks5-hostname 127.0.0.1:10801 http://<internal-host>/
export ALL_PROXY=socks5h://127.0.0.1:10801
```

</details>

<details>
<summary><strong>Two-level topology</strong></summary>

```bash
./agent -s change-me -l 127.0.0.1:19138   # child, passive
```

```json
{ "name": "connect_node", "arguments": { "node_id": 0, "address": "127.0.0.1:19138" } }
```

</details>

<details>
<summary><strong>Forward / backward / upload</strong></summary>

```json
{
  "name": "start_forward",
  "arguments": {
    "node_id": 0,
    "listen_address": "127.0.0.1:19141",
    "target_address": "10.0.0.5:80"
  }
}
```

Connect to `listen_address` **on the agent host**.

```json
{
  "name": "start_backward",
  "arguments": {
    "node_id": 0,
    "local_address": "127.0.0.1:19142",
    "target_address": "10.0.0.5:80"
  }
}
```

Connect to `127.0.0.1:19142` **on the controller host**.

```json
{
  "name": "upload_file",
  "arguments": {
    "node_id": 0,
    "local_path": "/path/to/tool",
    "remote_path": "/tmp/tool"
  }
}
```

</details>

## CLI flags

<details>
<summary><strong>controller</strong></summary>

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret |
| `-l` | Listen for agents `[ip]:port` |
| `-c` | Optional active connect |
| `-down` | `raw` / `ws` |
| `-tls-enable` | TLS on node links |
| `-domain` | TLS SNI / WS domain |
| `-heartbeat` | Heartbeat to first node |

</details>

<details>
<summary><strong>agent</strong></summary>

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret |
| `-c` | Connect to parent / controller |
| `-l` | Passive listen |
| `-up` / `-down` | `raw` / `ws` |
| `-tls-enable` / `-domain` | TLS |
| `-reconnect` | Seconds (`0` = off) |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | Reach parent via SOCKS5 |
| `-http-proxy` | Reach parent via HTTP proxy |

</details>

Controller and agents must share the **same secret** (and matching TLS/WS options).

## Security notes

- Treat `-s` / `STYX_SECRET` like a password; rotate after shared labs.
- Bind SOCKS to `127.0.0.1` unless you intentionally expose it.
- Upload paths are sanitized; still only upload to hosts you control.
- Builds may log MCP stdio under `/tmp/styx-mcp-mcp.log` — don’t put secrets in tool args while logging is on.

## Project layout

```text
cmd/controller/     controller + MCP entrypoint
cmd/agent/          agent entrypoint
scripts/            Cursor MCP wrapper
pkg/controller/     control plane, SOCKS / backward
pkg/mcp/            MCP tools
pkg/node/           agent handlers
pkg/protocol/       wire protocol
pkg/share/preauth/  HMAC mutual preauth
```

## Acknowledgments

Multi-hop proxy design draws heavily from [Stowaway](https://github.com/ph4ntonn/Stowaway) (MIT, © ph4ntom).  
MCP server stack uses [mcp-go](https://github.com/mark3labs/mcp-go).

## License

[MIT](LICENSE)
