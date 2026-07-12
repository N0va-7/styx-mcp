# styx-mcp

MCP-facing multi-hop proxy inspired by [Stowaway](https://github.com/ph4ntonn/Stowaway).

It turns a tree of **agent** nodes into tools an LLM client (Cursor, Claude Desktop, etc.) can call: list topology, open SOCKS5, forward ports, and upload files — without a separate admin TUI.

## Disclaimer

**Authorized use only.** Use this software only on systems and networks you own or have explicit permission to test (labs, CTF / exam ranges, written RoE). Misuse against unauthorized targets is illegal. The authors are not responsible for misuse.

## Features

| Area | Capability |
| :--- | :--- |
| Topology | Tree of agents; active connect (`-c`) or passive listen (`-l`) |
| Auth | Shared secret with mutual HMAC challenge-response preauth |
| Transport | Raw TCP; optional TLS; optional WebSocket (`raw` / `ws`) |
| SOCKS5 | Listen on **controller**; traffic exits via selected node (Stowaway-style) |
| Forward | Listen on **agent**, dial a target reachable from that agent |
| Backward | Listen on **controller**, dial target through a node |
| Files | Upload local file to a remote path on a node (path sanitized) |
| MCP | stdio MCP server; async tasks via `get_task_status` |
| Build | Cross-compile: `linux-amd64`, `windows-amd64`, `darwin-arm64` |

**Not included (yet):** remote interactive shell, download-from-node, SOCKS auth, full Stowaway admin UI.

## Architecture

```text
LLM client (Cursor / Claude …)
        │  MCP over stdio
        ▼
   controller  ──listen──►  agent (node 0)
        │                      │
   SOCKS / backward         start_listener / connect_node
   listen here                 ▼
                            agent (node 1) …
```

- **controller**: control plane + MCP tools. Agents connect here. SOCKS / reverse-forward listeners bind on the controller host.
- **agent**: runs on a foothold; dials out or listens for children; executes SOCKS dials / forwards / uploads at the edge.

## Build

Requires Go 1.21+ (see `go.mod` for the exact toolchain used in this tree).

```bash
# current platform (darwin-arm64 helpers in Makefile)
make build

# all release targets
make build-all

make test
```

Binaries land under `release/<os>-<arch>/` (`controller`, `agent`). That directory is gitignored; build locally or attach artifacts in GitHub Releases.

## Cursor MCP setup

1. Build for your Mac/Linux host so `release/<os>-<arch>/controller` exists.
2. Add to `~/.cursor/mcp.json` (or project `.cursor/mcp.json`):

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

3. Cursor Settings → MCP → enable / refresh **styx-mcp**.
4. On a foothold, run a matching **agent** (same secret):

```bash
./agent -s change-me -c <controller-reachable-ip>:19137
```

Wrapper env vars:

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `STYX_SECRET` | `secret` | Shared secret (`-s`) |
| `STYX_LISTEN` | `0.0.0.0:19137` | Agent listen address on controller (`-l`) |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | Controller stderr append log |
| `STYX_BIN_DIR` | `release/<os>-<arch>` under repo | Override binary directory |

**Do not commit real secrets** into public repos or shared `mcp.json` examples.

### Manual controller (without wrapper)

Controller must keep **stdio** free for MCP. Prefer the wrapper under Cursor. For debugging you can still run:

```bash
./release/darwin-arm64/controller -s change-me -l 0.0.0.0:19137
```

Logs default to `/tmp/styx-mcp-controller.log` (MCP traffic may also be mirrored under `/tmp/styx-mcp-mcp.log` depending on build).

## Quick start (CLI smoke test)

```bash
# terminal A — only needed if not using Cursor MCP wrapper
./release/darwin-arm64/controller -s change-me -l 127.0.0.1:19137

# terminal B
./release/darwin-arm64/agent -s change-me -c 127.0.0.1:19137
```

Then from the MCP client: `list_nodes` → expect node `0`.

## MCP tools

| Tool | Purpose | Where it listens / acts |
| :--- | :--- | :--- |
| `list_nodes` | Show topology | — |
| `get_node_detail` | Node detail | — |
| `add_node_memo` / `delete_node_memo` | Annotate a node | — |
| `start_listener` | Node listens for child agents | **Agent** |
| `connect_node` | Node dials a child agent address | **Agent** dials out |
| `start_socks` | SOCKS5 for local tools | **Controller** listen; exit via `node_id` |
| `start_forward` | Local forward on a node | **Agent** listen → dial `target_address` |
| `start_backward` | Reverse forward | **Controller** listen → via node → target |
| `upload_file` | Push file to node | Controller → agent path |
| `get_task_status` | Poll async task | — |
| `shutdown_node` | Stop a node | — |

Long-running actions return a `task_id`; poll with `get_task_status`.

### Choosing SOCKS vs forward vs backward

| Goal | Use |
| :--- | :--- |
| Run `curl` / scanners on the **controller machine** against an internal network | `start_socks` |
| Map one **controller** port to one internal `ip:port` | `start_backward` |
| Open a port **on the foothold** that dials somewhere else | `start_forward` |

`start_forward` is **not** a substitute for local SOCKS.

## Examples

### Two-level topology

Passive child:

```bash
./agent -s change-me -l 127.0.0.1:19138
```

From MCP:

```json
{
  "name": "connect_node",
  "arguments": { "node_id": 0, "address": "127.0.0.1:19138" }
}
```

Or have node 0 listen and the child connect upward — same end state: `list_nodes` shows parent/child.

### SOCKS5

```json
{
  "name": "start_socks",
  "arguments": { "node_id": 0, "address": "127.0.0.1:10801" }
}
```

```bash
curl --socks5-hostname 127.0.0.1:10801 http://<internal-host>/
# or: ALL_PROXY=socks5h://127.0.0.1:10801
```

### Port forward (on agent)

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

Connect to `listen_address` **on that agent host**.

### Reverse port forward (on controller)

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

Connect to `127.0.0.1:19142` on the controller host.

### File upload

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

## Command-line flags

### controller

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret (required in practice) |
| `-l` | Listen for agents `[ip]:port` |
| `-c` | Optional active connect to a peer |
| `-down` | Downstream transport: `raw` / `ws` |
| `-tls-enable` | TLS for node links |
| `-domain` | TLS SNI / WebSocket domain |
| `-heartbeat` | Heartbeat to first node |

### agent

| Flag | Description |
| :--- | :--- |
| `-s` | Shared secret |
| `-c` | Active mode: connect to controller/parent |
| `-l` | Passive mode: listen for parent/children |
| `-up` / `-down` | Upstream / downstream: `raw` / `ws` |
| `-tls-enable` | TLS |
| `-domain` | TLS SNI / WebSocket domain |
| `-reconnect` | Reconnect interval seconds (`0` = off) |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | Reach parent via SOCKS5 |
| `-http-proxy` | Reach parent via HTTP proxy |

Controller and agents must use the **same secret** (and matching TLS/WS settings when enabled).

## Project layout

```text
cmd/controller/     controller + MCP entrypoint
cmd/agent/          agent entrypoint
scripts/            Cursor MCP wrapper
pkg/controller/     control plane, SOCKS/backward services
pkg/mcp/            MCP tool registration & handlers
pkg/node/           agent protocol handlers
pkg/protocol/       wire protocol
pkg/share/preauth/  HMAC mutual preauth
pkg/crypto/         AES + gzip helpers
pkg/tasks/          async task manager
pkg/topology/       node graph
pkg/transport/      TLS helpers
```

## Security notes

- Treat `-s` / `STYX_SECRET` like a password; rotate after shared exercises.
- Prefer binding SOCKS to `127.0.0.1` unless you intentionally expose it.
- Upload paths are sanitized against traversal; still only upload to hosts you control.
- MCP stdio logging (if enabled in your build) may record tool arguments — avoid pasting secrets into tool fields when logging is on.

## Acknowledgments

Protocol and multi-hop proxy ideas draw heavily from [Stowaway](https://github.com/ph4ntonn/Stowaway) (MIT, © ph4ntom). styx-mcp adds an MCP control plane and controller-side SOCKS semantics for LLM clients.

## License

MIT — see [`LICENSE`](LICENSE).
