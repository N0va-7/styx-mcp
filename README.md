# styx-mcp

A Go-based MCP (Model Context Protocol) proxy network inspired by [Stowaway](https://github.com/ph4ntonn/Stowaway). It exposes a network of agent nodes as MCP tools, allowing LLM clients to build topologies, run SOCKS5 proxies, and transfer files through controlled nodes.

## Architecture

- **controller**: MCP server + control plane. Accepts node connections and exposes MCP stdio tools.
- **node**: Agent runtime. Connects to the controller or another node to form a tree topology.
- **MCP tools**: JSON-RPC tools callable by any MCP client.

## Build

Requires Go 1.21+.

```bash
make build
```

Binaries are written to `release/`.

## Quick Start

### 1. Start the controller

```bash
./release/styx-mcp-controller -s mysecret -l 127.0.0.1:19137
```

The controller listens for node connections on `127.0.0.1:19137` and speaks MCP over stdio.

### 2. Connect the first node

```bash
./release/styx-mcp-node -s mysecret -c 127.0.0.1:19137
```

### 3. Use MCP tools

From an MCP client (e.g., Claude Desktop, or the Python test scripts under `/tmp`):

- `list_nodes` — show topology
- `get_node_detail` — show node details
- `start_listener` — ask a node to listen for child connections
- `connect_node` — ask a node to connect to a child
- `start_socks` — run a SOCKS5 proxy on a node
- `upload_file` — upload a file to a node
- `get_task_status` — query async task status
- `shutdown_node` — terminate a node

## Example: Two-Level Topology

Start a second node in passive mode:

```bash
./release/styx-mcp-node -s mysecret -l 127.0.0.1:19138
```

Then call MCP tools:

```json
{"method": "tools/call", "params": {"name": "connect_node", "arguments": {"node_id": 0, "address": "127.0.0.1:19138"}}}
```

`list_nodes` will now show two nodes with a parent-child relationship.

## Example: SOCKS5 Proxy

```json
{"method": "tools/call", "params": {"name": "start_socks", "arguments": {"node_id": 0, "address": "127.0.0.1:19139"}}}
```

Then configure your application to use SOCKS5 proxy `127.0.0.1:19139` (traffic is forwarded through node 0).

## Example: Port Forward

Forward a port on a node to a target address accessible by that node:

```json
{"method": "tools/call", "params": {"name": "start_forward", "arguments": {"node_id": 0, "listen_address": "127.0.0.1:19141", "target_address": "127.0.0.1:19140"}}}
```

Traffic to `127.0.0.1:19141` on node 0 is forwarded to `127.0.0.1:19140`.

## Example: Reverse Port Forward

Listen on the controller machine and forward traffic through a node to a target:

```json
{"method": "tools/call", "params": {"name": "start_backward", "arguments": {"node_id": 0, "local_address": "127.0.0.1:19142", "target_address": "127.0.0.1:19140"}}}
```

Traffic to `127.0.0.1:19142` on the controller is forwarded through node 0 to `127.0.0.1:19140`.

## Example: File Upload

```json
{"method": "tools/call", "params": {"name": "upload_file", "arguments": {"node_id": 0, "local_path": "/tmp/source.txt", "remote_path": "/tmp/destination.txt"}}}
```

## Command-Line Flags

### controller

- `-s string`: shared secret for node communication
- `-l string`: listen address for nodes (`[ip]:<port>`)
- `-tls-enable`: enable TLS for node communication

### node

- `-s string`: shared secret
- `-c string`: active mode target controller/node address (`<ip>:<port>`)
- `-l string`: passive mode listen address (`[ip]:<port>`)
- `-tls-enable`: enable TLS

## Project Layout

```
cmd/
  controller/    controller entrypoint
  node/          node entrypoint
internal/utils/  helpers
pkg/
  controller/    control plane
  crypto/        AES + gzip
  mcp/           MCP server & tools
  node/          agent runtime
  protocol/      wire protocol
  share/preauth/ pre-authentication
  tasks/         async task manager
  topology/      node topology store
  transport/     TLS helpers
```

## License

MIT
