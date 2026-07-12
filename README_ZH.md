# styx-mcp

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![MCP](https://img.shields.io/badge/MCP-stdio-black)](https://modelcontextprotocol.io/)
[![GitHub stars](https://img.shields.io/github/stars/N0va-7/styx-mcp?style=social)](https://github.com/N0va-7/styx-mcp/stargazers)

[English](README.md) | **简体中文**

> 用 **MCP 工具** 控制的多级代理 —— 给 Cursor / Claude 一套 Stowaway 风格的跳板网络，不必再开 admin 交互界面。

```text
LLM (Cursor)  --MCP/stdio-->  controller  <-->  agent  <-->  agent …
                                   |                 |
                              SOCKS / 反向转发      流量出口
```

## 声明

> **仅限授权使用。** 只能用于你拥有或已获书面授权的目标（实验室、CTF/考试靶场、明确 RoE 的项目）。对未授权目标使用可能违法。使用后果由使用者自行承担。

**使用前请完整阅读本文（尤其是 [SOCKS / forward / backward 怎么选](#socks--forward--backward-怎么选)）。**

## 为什么是 styx-mcp？

| | [Stowaway](https://github.com/ph4ntonn/Stowaway) | **styx-mcp** |
| :--- | :--- | :--- |
| 控制面 | 交互式 admin | **MCP 工具**（LLM / Cursor 原生） |
| SOCKS 监听 | 管理端 | **Controller**（同一思路） |
| 主要使用者 | 人工操作 | Agent + 人工 |
| 远程 shell / 下载 | 有 | 暂无 |

借鉴 Stowaway 的多级代理模型，面向 **Model Context Protocol** 客户端重做控制面。

## 特性

- 树形拓扑：主动（`-c`）/ 被动（`-l`），多级跳转
- 双向 **HMAC** 预认证；可选 **TLS** / WebSocket
- **SOCKS5** 开在 controller，本机工具经指定节点出站
- 每流 **字节窗口流控**（禁止静默丢 SOCKS 数据；controller/agent 需同版本）
- **Forward**（agent 监听）与 **Backward**（controller 监听）
- **异步 `run_command`**（非交互 `sh -c`，返回 `task_id`）
- **异步 `download_file`** / `upload_file`（任务 + 本地路径；非交互 shell）
- 向节点 **上传** 文件（路径防穿越）
- 异步任务 + `get_task_status`
- 交叉编译：Linux / Windows / macOS（`make build-all`）

<details>
<summary><strong>暂未包含</strong></summary>

- 交互式远程 shell
- 从节点下载到 controller
- SOCKS 用户名密码认证
- 完整 Stowaway 式 admin UI

</details>

## 目录

- [快速开始](#快速开始)
- [接入 Cursor MCP](#接入-cursor-mcp)
- [MCP 工具](#mcp-工具)
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
        "STYX_SECRET": "change-me",
        "STYX_LISTEN": "0.0.0.0:19137",
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
| `STYX_SECRET` | `secret` | 预共享密钥（`-s`） |
| `STYX_LISTEN` | `0.0.0.0:19137` | controller 等待 agent 的地址 |
| `STYX_LOG` | `/tmp/styx-mcp-controller.log` | controller 日志 |
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
| `download_file` | 下载 | Agent → controller 路径 |
| `run_command` | 非交互命令 | Agent `sh -c`（异步 `task_id`） |
| `get_task_status` | 查异步任务 | — |
| `shutdown_node` | 关闭节点 | — |

耗时操作返回 `task_id`，用 `get_task_status` 轮询。

### SOCKS / forward / backward 怎么选

| 你想… | 用 |
| :--- | :--- |
| 在 **controller 本机** 用 `curl`/扫描器打内网 | `start_socks` |
| **controller** 某一端口映射到内网 `ip:port` | `start_backward` |
| 在 **落脚点** 开端口再转到别处 | `start_forward` |

`start_forward` **不能** 当成给本机用的 SOCKS。

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

## 命令行参数

<details>
<summary><strong>controller</strong></summary>

| 参数 | 说明 |
| :--- | :--- |
| `-s` | 预共享密钥 |
| `-l` | 等待 agent：`[ip]:port` |
| `-c` | 可选主动连接 |
| `-down` | `raw` / `ws` |
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
| `-up` / `-down` | `raw` / `ws` |
| `-tls-enable` / `-domain` | TLS |
| `-reconnect` | 重连间隔秒（`0` = 关） |
| `-socks5-proxy` / `-socks5-proxyu` / `-socks5-proxyp` | 经 SOCKS5 连父节点 |
| `-http-proxy` | 经 HTTP 代理连父节点 |

</details>

controller 与 agent 必须使用 **相同密钥**（TLS/WS 配置也要一致）。

## 安全建议

- 把 `-s` / `STYX_SECRET` 当密码；演练后轮换。
- SOCKS 尽量绑 `127.0.0.1`，除非有意对外暴露。
- 上传路径已防穿越，仍只应上传到你控制的主机。
- 构建可能把 MCP stdio 记到 `/tmp/styx-mcp-mcp.log` —— 开日志时不要把密钥填进工具参数。

## 目录结构

```text
cmd/controller/     controller + MCP 入口
cmd/agent/          agent 入口
scripts/            Cursor MCP wrapper
pkg/controller/     控制面、SOCKS / backward
pkg/mcp/            MCP 工具
pkg/node/           agent 协议处理
pkg/protocol/       线协议
pkg/share/preauth/  HMAC 双向预认证
```

## 致谢

多级代理思路大量参考 [Stowaway](https://github.com/ph4ntonn/Stowaway)（MIT，© ph4ntom）。  
MCP 服务端基于 [mcp-go](https://github.com/mark3labs/mcp-go)。

## 许可证

[MIT](LICENSE)
