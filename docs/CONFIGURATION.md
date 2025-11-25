# Gateway Configuration Guide

## Overview

Gateway configuration is strictly separated into two categories with **single source of truth**:

| Category | Source | Fallback | If Missing |
|----------|--------|----------|------------|
| **Business Config** | Redis ONLY | None | Gateway exits |
| **Infrastructure Config** | Env Vars / ConfigMap | Defaults | Uses defaults |

## Configuration Categories

### 1. Business Configuration (业务配置) - Redis Only

**Source: Redis ONLY. No fallback. Gateway is READ-ONLY.**

Business configuration is managed by external admin tools (web console, CLI, etc.). Gateway only reads from Redis.

| Key | Description | Required |
|-----|-------------|----------|
| `server.listen_addr` | Gateway listening address (e.g., `:8080`) | YES |
| `server.max_connections` | Maximum concurrent connections | NO (default: 10000) |
| `backends.http.target_url` | HTTP backend URL | YES |
| `backends.http.timeout` | HTTP request timeout | NO (default: 30s) |
| `backends.tcp.target_addr` | TCP backend address | YES |
| `backends.tcp.timeout` | TCP connection timeout | NO (default: 60s) |
| `lifecycle.shutdown_timeout` | Graceful shutdown timeout | NO (default: 60s) |
| `lifecycle.drain_wait_time` | Drain wait for long connections | NO (default: 3600s) |

**Redis Key:** `gateway:business:config` (Hash)

**Behavior:**
- Redis unavailable → Gateway exits with error
- Config missing → Gateway exits with error
- Config incomplete (missing required fields) → Gateway exits with error

---

### 2. Security Configuration (安全配置) - Redis Only

**Source: Redis ONLY. Gateway is READ-ONLY.**

| Key | Description | Redis Key |
|-----|-------------|-----------|
| Rate Limit | QPS limiting | `gateway:rate_limit` (Hash) |
| WAF Blocked IPs | IP blacklist | `gateway:waf:blocked_ips` (Set) |
| WAF Blocked Patterns | URL pattern blacklist | `gateway:waf:blocked_patterns` (Set) |
| Auth Subjects | Allowed client subjects | `gateway:auth:allowed_subjects` (Set) |

**Important: Uses Redis Set for atomic operations**

External admin tools use `SADD`/`SREM` for IP and pattern management. This prevents multiple gateway instances from overwriting each other's changes.

---

### 3. Infrastructure Configuration (基础配置) - Env Vars

**Source: Environment Variables / ConfigMap. Has defaults.**

| Env Variable | Description | Default |
|--------------|-------------|---------|
| `METRICS_ENABLED` | Enable Prometheus metrics | `true` |
| `METRICS_LISTEN_ADDR` | Metrics server address | `:9090` |
| `REDIS_ENABLED` | Enable Redis (required) | `true` |
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password | `` |
| `REDIS_DB` | Redis database number | `0` |
| `REDIS_KEY_PREFIX` | Redis key prefix | `gateway:` |

---

## Redis Data Structure

### Business Config (Hash)
```
gateway:business:config
├── server.listen_addr = ":8080"
├── server.max_connections = "10000"
├── backends.http.target_url = "http://httpproxy:5000"
├── backends.http.timeout = "30s"
├── backends.tcp.target_addr = "gateserver:6000"
├── backends.tcp.timeout = "60s"
├── lifecycle.shutdown_timeout = "60s"
└── lifecycle.drain_wait_time = "3600s"
```

### Security Config
```
gateway:rate_limit (Hash)
├── enabled = "true"
├── rps = "100"
└── burst = "200"

gateway:waf:blocked_ips (Set)
├── "1.2.3.4"
├── "5.6.7.8"
└── ...

gateway:waf:blocked_patterns (Set)
├── "/admin/*"
├── "*.php"
└── ...

gateway:auth:allowed_subjects (Set)
├── "client-a"
├── "client-b"
└── ...
```

---

## Admin Operations (External Tools)

Gateway is READ-ONLY. Use these Redis commands from external admin tools:

### Initialize Business Config
```bash
redis-cli HSET gateway:business:config \
  server.listen_addr ":8080" \
  server.max_connections "10000" \
  backends.http.target_url "http://httpproxy:5000" \
  backends.http.timeout "30s" \
  backends.tcp.target_addr "gateserver:6000" \
  backends.tcp.timeout "60s" \
  lifecycle.shutdown_timeout "60s" \
  lifecycle.drain_wait_time "3600s"
```

### Update Rate Limit
```bash
redis-cli HSET gateway:rate_limit enabled true rps 100 burst 200
```

### Add Blocked IP (Atomic, no overwrite)
```bash
redis-cli SADD gateway:waf:blocked_ips "1.2.3.4" "5.6.7.8"
```

### Remove Blocked IP (Atomic, no overwrite)
```bash
redis-cli SREM gateway:waf:blocked_ips "1.2.3.4"
```

### List Blocked IPs
```bash
redis-cli SMEMBERS gateway:waf:blocked_ips
```

### Notify Gateways of Config Change (Hot Reload)
```bash
redis-cli PUBLISH gateway:config:changed '{"type":"security"}'
```

---

## Readiness Probe Behavior

| Condition | /health | /ready | K8s Action |
|-----------|---------|--------|------------|
| Normal | 200 | 200 | Traffic routed |
| Redis unavailable | 200 | 503 | Removed from endpoints |
| Draining (shutdown) | 200 | 503 | Removed from endpoints |

---

## Startup Flow

```
1. Load Infrastructure Config (Env Vars)
   ↓
2. Connect to Redis
   ├── Failed → EXIT(1)
   └── Success ↓
3. Load Business Config from Redis
   ├── Missing → EXIT(1)
   └── Success ↓
4. Load Security Config from Redis
   ├── Missing → Use defaults
   └── Success ↓
5. Start Metrics Server (:9090)
   ↓
6. Start Business Listener (:8080)
   ↓
7. Wait for SIGTERM
   ↓
8. Graceful Shutdown (Drain Mode)
```

---

## Hot Reload

Gateway subscribes to `gateway:config:changed` channel for hot reload.

When admin tools update config, publish a notification:
```bash
redis-cli PUBLISH gateway:config:changed '{"type":"security"}'
```

Gateway will reload security config without restart.

---

## Best Practices

1. **Always configure Redis first** before starting gateway
2. **Use Set (`SADD`/`SREM`) for IP/pattern lists** to prevent multi-instance conflicts
3. **Use Hash for structured config** (business, rate_limit)
4. **Publish config changes** for hot reload
5. **Monitor `/ready` endpoint** for Redis health

---

## Troubleshooting

### Gateway exits immediately
```
CRITICAL: Failed to connect to Redis
CRITICAL: Failed to load business config from Redis
```
**Solution:** Configure Redis and set business config first.

### Gateway not ready (503)
```bash
curl http://localhost:9090/ready
# Redis Unavailable: connection refused
```
**Solution:** Check Redis connection.

### Config not updating
**Solution:** Publish change notification:
```bash
redis-cli PUBLISH gateway:config:changed '{"type":"security"}'
```
