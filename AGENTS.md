# styx-mcp

MCP 多级跳板：controller 走 stdio MCP，agent 在对端出流量。  
多跳思路致谢 [Stowaway](https://github.com/ph4ntonn/Stowaway)（MIT）；线协议身份、加解密与工具面是本仓库的。仅限授权环境。

## Dev environment tips

- Go 版本见 `go.mod`。仓库根目录：`go test ./...`、`go build ./...`。
- 本机产物：`make build` → `release/<os>-<arch>/`。交叉编译：`make build-all`。
- MCP 入口：`scripts/styx-mcp-wrapper.sh`。配置模板：`.grok/config.toml.example`。
- 密钥：`STYX_SECRET` 或 gitignore 的 `.grok/styx.secret`，不要提交真实密钥。
- agent 默认连 `19137`（`STYX_LISTEN`）。该端口同时只跑一个 controller；占用就换地址或先释放。
- 改握手 / 加密 / 组帧后，controller 与 agent 用**同一 commit** 构建再联调。
- `start_socks` 在 **controller 本机**听，流量经 `node_id` 出；`start_forward` 在 **agent** 上听，不是本机 SOCKS 替代品。
- 布局：`cmd/{controller,agent}`、`pkg/mcp`、`pkg/controller`、`pkg/node`、`pkg/protocol`、`pkg/topology`（`Topology.Do`）。

## Testing instructions

- 对改动相关包跑 `go test ./...`，修到绿。
- **改什么测什么**：正常路径 + 一两个边界；深度跟风险走。
- 拓扑：列表 / 详情 / memo；下线后稀疏 ID；并发读不串。
- listen / 启动：能绑定；端口占用要失败且错误可读。
- SOCKS：经代理能到目标；本机直连内网可能不通。
- 纯逻辑优先单测；跨进程行为用 MCP + 真 agent。
- 全链路靶场（入口 → agent → 代理 → 内网）可选，大改或用户要求时再做，不是每次固定剧本。
- 为本次改动补测，即使没人点名要求。

## PR instructions

- 相关检查通过再 commit；用户没说就不要 `git push`。
- 说明：`fix:` / `feat:` / `refactor:` / `docs:`，短标题，必要时写 why。
- 不要提交 `release/`、密钥、本地 `.grok/config.toml`。
