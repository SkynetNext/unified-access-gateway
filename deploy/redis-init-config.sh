#!/bin/bash
# Initialize Gateway Business Configuration in Redis
# Run this script BEFORE deploying the gateway

set -e

REDIS_HOST="${REDIS_HOST:-10.1.0.8}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
REDIS_PREFIX="${REDIS_PREFIX:-uag:}"

# Redis CLI command
if [ -n "$REDIS_PASSWORD" ]; then
    REDIS_CLI="redis-cli -h $REDIS_HOST -p $REDIS_PORT -a $REDIS_PASSWORD"
else
    REDIS_CLI="redis-cli -h $REDIS_HOST -p $REDIS_PORT"
fi

echo "==> Initializing Gateway Business Configuration"
echo "Redis: $REDIS_HOST:$REDIS_PORT"
echo "Prefix: $REDIS_PREFIX"
echo ""

# ============================================
# Business Configuration
# ============================================

echo "==> Setting Business Config..."

$REDIS_CLI HSET "${REDIS_PREFIX}business:config" \
    "server.listen_addr" ":8080" \
    "server.max_connections" "10000" \
    "backends.http.target_url" "http://httpproxy-service.hgame.svc.cluster.local:5000" \
    "backends.http.timeout" "30s" \
    "backends.tcp.target_addr" "hgame-gategame-backend-1.hgame.svc.cluster.local:9621" \
    "backends.tcp.timeout" "60s" \
    "lifecycle.shutdown_timeout" "60s" \
    "lifecycle.drain_wait_time" "3600s"

echo "✅ Business config set"

# ============================================
# Security Configuration
# ============================================

echo "==> Setting Security Config..."

# Auth config
$REDIS_CLI HSET "${REDIS_PREFIX}auth:config" \
    "enabled" "false" \
    "header_subject" "X-Client-Subject"

# Rate limit config
$REDIS_CLI HSET "${REDIS_PREFIX}rate_limit" \
    "enabled" "true" \
    "rps" "1000" \
    "burst" "2000"

# WAF config
$REDIS_CLI HSET "${REDIS_PREFIX}waf:config" \
    "enabled" "true"

# WAF blocked IPs (empty initially)
# $REDIS_CLI SADD "${REDIS_PREFIX}waf:blocked_ips" "1.2.3.4"

# WAF blocked patterns
$REDIS_CLI SADD "${REDIS_PREFIX}waf:blocked_patterns" \
    "(?i)(union.*select)" \
    "(?i)(insert.*into)" \
    "(?i)(<script>)" \
    "(?i)(javascript:)"

echo "✅ Security config set"

# ============================================
# Verify Configuration
# ============================================

echo ""
echo "==> Verifying Configuration..."

echo "Business Config:"
$REDIS_CLI HGETALL "${REDIS_PREFIX}business:config"

echo ""
echo "Auth Config:"
$REDIS_CLI HGETALL "${REDIS_PREFIX}auth:config"

echo ""
echo "Rate Limit Config:"
$REDIS_CLI HGETALL "${REDIS_PREFIX}rate_limit"

echo ""
echo "WAF Config:"
$REDIS_CLI HGETALL "${REDIS_PREFIX}waf:config"

echo ""
echo "WAF Blocked Patterns:"
$REDIS_CLI SMEMBERS "${REDIS_PREFIX}waf:blocked_patterns"

echo ""
echo "✅ Gateway configuration initialized successfully!"
echo ""
echo "To update configuration dynamically, publish to channel:"
echo "  PUBLISH ${REDIS_PREFIX}config:changed '{\"type\":\"rate_limit\",\"data\":{}}'"

