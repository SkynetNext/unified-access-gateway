# Security Features

Unified Access Gateway provides core security capabilities including authentication, rate limiting, WAF, and audit logging.

## Features

### 1. Authentication (HTTPS Certificate Verification)

The gateway can verify client TLS certificates and validate subjects against an allowlist.

**Configuration:**
```yaml
security:
  auth:
    enabled: true
    header_subject: "X-Client-Subject"  # Fallback header if TLS cert not present
    allowed_subjects:                    # List of allowed certificate subjects
      - "CN=client1.example.com"
      - "CN=client2.example.com"
```

**Environment Variables:**
- `AUTH_ENABLED=true`
- `AUTH_HEADER_SUBJECT=X-Client-Subject`
- `AUTH_ALLOWED_SUBJECTS=CN=client1,CN=client2`

**How it works:**
- For HTTPS connections, the gateway extracts the client certificate subject from `r.TLS.PeerCertificates[0].Subject`
- If no TLS certificate is present, it falls back to checking the header specified in `header_subject`
- Only requests with subjects in the `allowed_subjects` list are allowed

### 2. Rate Limiting

Token bucket rate limiter to prevent abuse and control traffic.

**Configuration:**
```yaml
security:
  rate_limit:
    enabled: true
    requests_per_second: 100.0  # RPS limit
    burst: 200                  # Burst capacity
```

**Environment Variables:**
- `RATE_LIMIT_ENABLED=true`
- `RATE_LIMIT_RPS=100`
- `RATE_LIMIT_BURST=200`

**Dynamic Updates via Admin API:**
```bash
curl -X POST http://localhost:9090/admin/security/rate-limit \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "requests_per_second": 200, "burst": 400}'
```

### 3. Web Application Firewall (WAF)

IP-based blocking and pattern-based request filtering.

**Configuration:**
```yaml
security:
  waf:
    enabled: true
    blocked_ips:
      - "192.168.1.100"
      - "10.0.0.5"
    blocked_patterns:
      - "(?i)(union|select|drop|delete|insert|update).*from"
      - "(?i)(<script|javascript:|onerror=)"
```

**Environment Variables:**
- `WAF_ENABLED=true`
- `WAF_BLOCKED_IPS=192.168.1.100,10.0.0.5`
- `WAF_BLOCKED_PATTERNS=(?i)(union|select).*from,(?i)<script`

**Dynamic Updates via Admin API:**

Add blocked IPs:
```bash
curl -X POST http://localhost:9090/admin/security/waf/ips \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "ips": ["192.168.1.200"]}'
```

Remove blocked IPs:
```bash
curl -X POST http://localhost:9090/admin/security/waf/ips \
  -H "Content-Type: application/json" \
  -d '{"action": "remove", "ips": ["192.168.1.100"]}'
```

Update patterns:
```bash
curl -X POST http://localhost:9090/admin/security/waf/patterns \
  -H "Content-Type: application/json" \
  -d '{"action": "add", "patterns": ["(?i)malicious"]}'
```

### 4. Audit Logging

Comprehensive audit trail for all requests (allow/deny decisions).

**Configuration:**
```yaml
security:
  audit:
    enabled: true
    sink: "stdout"  # Options: stdout, stderr, file:///var/log/gateway/audit.log
```

**Environment Variables:**
- `AUDIT_ENABLED=true`
- `AUDIT_SINK=stdout`  # or `file:///var/log/gateway/audit.log`

**Audit Log Format:**

HTTP:
```json
{"ts":"2024-01-01T12:00:00Z","protocol":"http","remote_addr":"192.168.1.1:54321","method":"GET","path":"/api/users","status":200,"action":"allow","duration_ms":45,"detail":""}
```

TCP:
```json
{"ts":"2024-01-01T12:00:00Z","protocol":"tcp","remote_addr":"192.168.1.1:54321","backend":"gateserver-service:6000","action":"allow","detail":""}
```

Denied requests include error details:
```json
{"ts":"2024-01-01T12:00:00Z","protocol":"http","remote_addr":"192.168.1.100:54321","method":"GET","path":"/api/users","status":403,"action":"deny","duration_ms":1,"detail":"blocked IP: 192.168.1.100"}
```

## Control Plane API

The Admin API runs on the metrics port (`:9090` by default) and provides endpoints for dynamic configuration:

- `GET /admin/config` - Get current security configuration
- `POST /admin/security/rate-limit` - Update rate limit settings
- `POST /admin/security/waf/ips` - Add/remove blocked IPs
- `POST /admin/security/waf/patterns` - Add/remove blocked patterns
- `GET /admin/health` - Admin API health check

**Example: Get current config**
```bash
curl http://localhost:9090/admin/config
```

## Integration Points

### HTTP Handler Integration

The security manager is integrated into the HTTP handler:
- Connection-level checks (IP blocking, rate limiting) happen before accepting connections
- HTTP-level checks (WAF patterns, auth) happen during request processing
- Audit logs are written for all requests (allow/deny)

### TCP Handler Integration

For TCP connections:
- Connection-level checks (IP blocking, rate limiting) happen before proxying
- Audit logs record connection attempts and outcomes

## Best Practices

1. **Enable audit logging in production** for compliance and security analysis
2. **Use file-based audit sink** (`file:///path/to/audit.log`) for persistent storage
3. **Configure rate limits** based on expected traffic patterns
4. **Regularly update WAF patterns** to block new attack vectors
5. **Monitor audit logs** for suspicious patterns (high deny rates, repeated blocked IPs)

## Security Considerations

- The Admin API endpoints are **not authenticated** by default. In production, you should:
  - Restrict access to the metrics port using network policies
  - Add authentication middleware to Admin API endpoints
  - Use Kubernetes RBAC to control access to the metrics service

- Rate limiting is **per-instance**. For distributed rate limiting, consider:
  - Using Redis-based distributed rate limiter
  - Configuring per-instance limits that sum to the desired global limit

- WAF patterns are **regex-based** and can impact performance. Keep patterns simple and test thoroughly.

