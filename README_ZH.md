<p align="center">
  <img src="docs/images/logo.png" alt="styx-mcp" width="520">
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white" alt="Go"></a>
  <a href="https://modelcontextprotocol.io/"><img src="https://img.shields.io/badge/MCP-stdio-black" alt="MCP"></a>
  <a href="https://github.com/N0va-7/styx-mcp/stargazers"><img src="https://img.shields.io/github/stars/N0va-7/styx-mcp?style=social" alt="GitHub stars"></a>
</p>

<p align="center"><a href="README.md">English</a> · <strong>简体中文</strong></p>

> 用 **MCP 工具** 编排多跳节点、隧道与内网侦察

<p align="center">
  <img src="docs/images/architecture.png" alt="styx-mcp 架构：LLM 与本机工具经 controller 连多级 agent" width="920">
</p>

## 声明

> **仅限授权使用。** 只能用于你拥有或已获书面授权的目标（实验室、CTF/考试靶场、明确 RoE 的项目）。对未授权目标使用可能违法。使用后果由使用者自行承担。

**使用前请完整阅读本文（尤其是 [SOCKS / forward / backward / scan 怎么选](#socks--forward--backward--scan-怎么选)）。**

## 特性

- 树形拓扑：主动（`-c`）/ 被动（`-l`），多级跳转
- 双向 **HMAC** 预认证；可选 **TLS**
- **SOCKS5** 开在 controller，本机工具经指定节点出站
- 每流 **字节窗口流控**（controller/agent 需同版本）
- **Forward**（agent 监听）与 **Backward**（controller 监听）
- **异步 `start_cmd`** — 一次性远程命令（`task_id`）
- **异步 `start_scan`** — 探活 → 端口 → 轻量指纹 + **refs**
- **异步 `pull_file`** / `upload_file`
- 异步任务 + `get_task_status`（阶段 phase；长扫描可带 `result.progress`）
- 交叉编译：Linux / Windows / macOS（`make build-all`）

## 目录

- [快速开始](#快速开始)
- [接入 Cursor MCP](#接入-cursor-mcp)
- [MCP 工具](#mcp-工具)
- [内网扫描（`start_scan`）](#内网扫描start_scan)
- [示例](#示例)
- [命令行参数](#命令行参数)
- [安全建议](#安全建议)
- [目录结构](#目录结构)
- [致谢](#致谢)
- [许可证](#许可证)

## 快速开始

```bash
git clone https://github.com/N0va-7/styx-mcp.git
cd styx-mcp
make build          # → release/<os>-<arch>/
```

```bash
# 终端 A — controller（stdio 留给 MCP；纯 CLI 冒烟才需要）
./release/$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')/controller \
  -s change-me -l 127.0.0.1:19137

# 终端 B — agent
./release/.../agent -s change-me -c 127.0.0.1:19137
```

用 **Cursor** 时：终端 A 可省略，按下面 [wrapper](#接入-cursor-mcp) 配置，只需在落脚点起 agent。

```bash
make build-all   # linux-amd64 / windows-amd64 / darwin-arm64
make test
```

## 接入 Cursor MCP

1. `make build`，确保有 `release/<os>-<arch>/controller`。
2. 写入 `~/.cursor/mcp.json`（或项目 `.cursor/mcp.json`）：

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

3. Cursor → **Settings → MCP** → 启用 / 刷新 **styx-mcp**。
4. 落脚点：

```bash
./agent -s change-me -c <controller的IP>:19137
```

| 环境变量 | 默认 | 含义 |
| :--- | :--- | :--- |
| `STYX_SECRET` | *（必填）* | 预共享密钥（`-s`） |
| `STYX_LISTEN` | `127.0.0.1:19137` | controller 等待 agent 的地址 |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | controller 日志 |
| `STYX_MCP_LOG` | *（关闭）* | 可选：记录原始 MCP stdio |
| `STYX_BIN_DIR` | `release/<os>-<arch>` | 覆盖二进制目录 |

不要把真实密钥提交到公开配置。

## MCP 工具

| 工具 | 作用 | 监听 / 作用位置 |
| :--- | :--- | :--- |
| `list_nodes` | 查看拓扑 | — |
| `get_node_detail` | 节点详情 | — |
| `add_node_memo` / `delete_node_memo` | 备注 | — |
| `start_listener` | 等待子 agent | **Agent** |
| `connect_node` | 主动连子节点 | **Agent** |
| `start_socks` | 本机 SOCKS5 | **Controller** → 经节点出站 |
| `start_forward` | 端口转发 | **Agent** 监听 → 目标 |
| `start_backward` | 反向转发 | **Controller** → 经节点 → 目标 |
| `upload_file` | 上传 | Controller → agent |
| `pull_file` | 拉文件到本机 | Agent → controller 路径 |
| `start_cmd` | 一次性远程命令 | Agent `sh -c`（异步 `task_id`） |
| `start_scan` | 内网端口扫描 + 轻量指纹 | **Agent**（异步 `task_id`） |
| `get_task_status` | 查异步任务 | — |
| `shutdown_node` | 关闭节点 | — |

耗时操作返回 `task_id`，用 `get_task_status` 轮询。

### SOCKS / forward / backward / scan 怎么选

| 你想… | 用 |
| :--- | :--- |
| 在 **controller 本机** 用 `curl`/扫描器打内网 | `start_socks` |
| **controller** 某一端口映射到内网 `ip:port` | `start_backward` |
| 在 **落脚点** 开端口再转到别处 | `start_forward` |
| 从 **agent 侧** 拿结构化开放端口 / 指纹 | `start_scan` |

<p align="center">
  <img src="docs/images/traffic-modes.png" alt="start_socks / backward / forward / start_scan 怎么选" width="920">
</p>

## 内网扫描（`start_scan`）

在选中的 **agent** 上执行（流量从该节点出）。

<p align="center">
  <img src="docs/images/scan-pipeline.png" alt="start_scan 流水线：探活、端口、指纹、refs" width="920">
</p>

**探活（混合，默认开）：** **ICMP 成功 或** 任一 TCP probe 口 open → 判 alive。  
若存活为 0，会 **降级扫描全部目标** 并写入 `warnings`（避免空结果被当成「网段没机器」）。

**端口方式：** `method=auto`（默认）在 agent 具备 raw IPv4 TCP（Linux root / CAP_NET_RAW）时用 **SYN**，否则 **TCP connect**。可强制 `connect` / `syn`。

| 参数 | 默认 | 说明 |
| :--- | :--- | :--- |
| `node_id` | 必填 | 经该 agent 出站 |
| `targets` | 必填 | IPv4：IP / CIDR / 逗号列表 |
| `mode` | `fast` | `fast` \| `normal` \| `full` \| `custom`（`full` 很重） |
| `ports` | — | `custom` 时必填（如 `22,80,8000-8100`） |
| `fingerprint` | `true` | 只指纹 open 端口 |
| `discover` | `true` | 混合探活 |
| `method` | `auto` | `auto` \| `connect` \| `syn` |
| `concurrency` | `200` | 上限 500 |
| `timeout_ms` | `500` | 单次探测超时 |

**阶段**（`get_task_status`）：`discovering` → `scanning` → `fingerprinting` → `done`。  
探活中 `result.progress` 可能含 `stage`、`icmp_done`/`icmp_total`、`icmp_alive`、`alive_n`、`tcp_probes`。

**同版本：** 改 SCAN\* 线协议后，controller 与 agent 须用 **同一 commit** 构建再联调。

授权靶场一键冒烟（监听 **19139**，不占 MCP 的 `:19137`）：

```bash
STYX_SECRET=… STYX_CALLBACK=<攻击机IP> ./scripts/lab-scan-e2e.sh
```

## 示例

<details open>
<summary><strong>SOCKS5</strong></summary>

```json
{ "name": "start_socks", "arguments": { "node_id": 0, "address": "127.0.0.1:10801" } }
```

```bash
curl --socks5-hostname 127.0.0.1:10801 http://<内网主机>/
export ALL_PROXY=socks5h://127.0.0.1:10801
```

</details>

<details>
<summary><strong>两级拓扑</strong></summary>

```bash
./agent -s change-me -l 127.0.0.1:19138   # 子节点被动监听
```

```json
{ "name": "connect_node", "arguments": { "node_id": 0, "address": "127.0.0.1:19138" } }
```

</details>

<details>
<summary><strong>Forward / Backward / 上传</strong></summary>

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

连接的是 **agent 主机** 上的 `listen_address`。

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

连接 **controller 主机** 上的 `127.0.0.1:19142`。

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
<summary><strong>内网扫描</strong></summary>

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

结果常用字段：`stats`、`open[]`、`summary.interesting[]`、可选 `warnings[]` / `refs`。

</details>

## 命令行参数

<details>
<summary><strong>controller</strong></summary>

| 参数 | 说明 |
| :--- | :--- |
| `-s` | 预共享密钥 |
| `-l` | 等待 agent：`[ip]:port` |
| `-c` | 可选主动连接 |
| `-down` | 仅 `raw`（`ws` 会拒绝） |
| `-tls-enable` | 节点链路 TLS |
| `-domain` | TLS SNI / WS domain |
| `-heartbeat` | 对首节点心跳 |

</details>

<details>
<summary><strong>agent</strong></summary>

| 参数 | 说明 |
| :--- | :--- |
| `-s` | 预共享密钥 |
| `-c` | 连接父节点 / controller |
| `-l` | 被动监听 |
| `-up` / `-down` | 仅 `raw`（`ws` 会拒绝） |
| `-tls-enable` / `-domain` | TLS |
| `-reconnect` | 重连间隔秒（`0` = 关） |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | 经 SOCKS5 连父节点 |
| `-http-proxy` | 经 HTTP 代理连父节点 |

</details>

controller 与 agent 必须使用 **相同密钥**（TLS/WS 配置也要一致）。

## 安全建议

- 把 `-s` / `STYX_SECRET` 当密码；演练后轮换。wrapper **必须**设置 `STYX_SECRET`（无弱默认值）。
- 载荷加密使用 **HKDF-SHA256** 派生的 AES-256-GCM 密钥（controller 与 agent 版本需一致）。
- 可选 TLS（`-tls-enable`）由共享密钥派生稳定证书并校验对端（仍需强密钥）。
- wrapper 默认监听 `127.0.0.1:19137`；远程 agent 时再设 `STYX_LISTEN=0.0.0.0:…`。
- SOCKS 尽量绑 `127.0.0.1`，除非有意对外暴露。
- 上传允许绝对路径、拒绝 `..`；单文件上限 32 MiB。
- MCP stdio 日志**默认关闭**；调试时再设 `STYX_MCP_LOG=/path`（可能含敏感信息）。
- 协议变更后，controller 与 agent 须用同一 commit 构建。

## 目录结构

```text
cmd/controller/     controller + MCP 入口
cmd/agent/          agent 入口
scripts/            MCP wrapper、lab-scan-e2e.sh、lab_scan_smoke.go
pkg/controller/     控制面、SOCKS / backward / scan 任务
pkg/mcp/            MCP 工具
pkg/node/           agent 协议处理（含 scan）
pkg/scan/           目标解析、探活、connect/SYN
pkg/fingerprint/    轻量指纹 + refs 表
pkg/protocol/       线协议
pkg/share/preauth/  HMAC 双向预认证
```

## 致谢

- [Stowaway](https://github.com/ph4ntonn/Stowaway)
- [fscan](https://github.com/shadow1ng/fscan)
- [mcp-go](https://github.com/mark3labs/mcp-go)

## 许可证

[MIT](LICENSE)
