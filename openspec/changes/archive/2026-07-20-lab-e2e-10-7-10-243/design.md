## Lab e2e 10.7.10.243 (2026-07-20)

### Chain
1. Edge `10.7.10.243` / `172.16.23.10` — EyouCMS (ThinkPHP) RCE → agent **node 0**
   `uuid=480c704604` user `www-data` hostname `4b81331d0c38`
2. Callback `192.168.230.57:19137` secret `ctfsecret` (Codex MCP controller)

### Feature matrix

| Feature | Result | Notes |
|---------|--------|-------|
| list_nodes | PASS | id=0 |
| get_node_detail | PASS | |
| add_node_memo | PASS | lab-10.7.10.243-e2e |
| start_cmd | PASS | id/hostname/ip |
| start_socks :11081 | PASS | ready; HTTP 200 闻道基金 via SOCKS |
| start_forward → 172.16.23.20:7001 | PASS | ready; via SOCKS HTTP 404 WebLogic |
| upload_file | PASS | 27 bytes |
| pull_file | PASS | 21 bytes + sha256 |
| start_scan fast hybrid | PASS | 23.1:22, 23.10:80, 23.20:7001, 23.1:80 |
| start_listener 127.0.0.1:19138 | PASS | ready |
| start_backward 17001→80 | PASS | HTTP 200 闻道基金 on controller |
| reconnect (lab TCP-drop) | SKIP | old `ss` no `-K`; unit/e2e cover |

### Decision
No functional blockers for release tag.
