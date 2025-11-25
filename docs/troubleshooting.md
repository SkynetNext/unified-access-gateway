# Troubleshooting

## Common Issues

### Gateway exits immediately

**Symptoms**:
```
CRITICAL: Failed to connect to Redis
CRITICAL: Failed to load business config from Redis
```

**Causes**:
- Redis not accessible
- Redis password incorrect
- Business configuration not initialized

**Solutions**:
1. Verify Redis connectivity:
   ```bash
   redis-cli -h 10.1.0.8 -p 6379 -a $REDIS_PASSWORD PING
   ```

2. Initialize business configuration:
   ```bash
   redis-cli -h 10.1.0.8 -a $REDIS_PASSWORD \
     HSET gateway:business:config \
     server.listen_addr ":8080" \
     backends.http.target_url "http://backend:5000" \
     backends.tcp.target_addr "backend:6000"
   ```

3. Check environment variables:
   ```bash
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   ```

### Gateway not ready (503)

**Symptoms**:
```bash
curl http://localhost:9090/ready
# Returns: 503 Service Unavailable
```

**Causes**:
- Redis unavailable
- Business configuration missing
- Gateway in drain mode

**Solutions**:
1. Check Redis health:
   ```bash
   redis-cli -h $REDIS_ADDR PING
   ```

2. Verify business configuration exists:
   ```bash
   redis-cli -h $REDIS_ADDR HGETALL gateway:business:config
   ```

3. Check gateway logs:
   ```bash
   kubectl logs <pod-name> -n uag | grep -i redis
   ```

### eBPF not working

**Symptoms**:
```
[INFO] eBPF not available, using userspace proxy
```

**Causes**:
- Kernel < 4.18
- Missing capabilities
- Cgroup v2 not mounted

**Solutions**:
1. Check kernel version:
   ```bash
   uname -r
   # Requires 4.18+
   ```

2. Check capabilities:
   ```bash
   kubectl describe pod <pod-name> -n uag | grep -i capability
   # Should include CAP_BPF
   ```

3. Verify cgroup v2:
   ```bash
   mount | grep cgroup2
   ```

**Note**: Gateway works fine without eBPF. It's an optimization, not a requirement.

### High latency

**Symptoms**:
- Request latency > 100ms
- P99 latency spikes

**Causes**:
- Backend service slow
- Network issues
- Resource constraints

**Solutions**:
1. Check backend health:
   ```bash
   curl http://backend:5000/health
   ```

2. Monitor metrics:
   ```bash
   curl http://localhost:9090/metrics | grep gateway_request_duration
   ```

3. Check resource usage:
   ```bash
   kubectl top pod <pod-name> -n uag
   ```

### Connection errors

**Symptoms**:
```
[ERROR] Accept error: use of closed network connection
```

**Causes**:
- Listener closed during shutdown
- Network interruption

**Solutions**:
- This is normal during graceful shutdown
- If persistent, check network connectivity

### Image pull errors

**Symptoms**:
```
ImagePullBackOff
pull access denied
```

**Solutions**:
1. Verify `imagePullSecrets` configured:
   ```bash
   kubectl get deployment uag -n uag -o yaml | grep imagePullSecrets
   ```

2. Create secret:
   ```bash
   kubectl create secret docker-registry harbor-credential \
     --docker-server=14.103.46.72 \
     --docker-username=<user> \
     --docker-password=<pass> \
     -n uag
   ```

### Graceful shutdown takes too long

**Symptoms**:
- Pod remains in `Terminating` state
- Shutdown exceeds `terminationGracePeriodSeconds`

**Causes**:
- Long-lived connections not closing
- `drain_wait_time` too high

**Solutions**:
1. Check active connections:
   ```bash
   curl http://localhost:9090/metrics | grep gateway_active_connections
   ```

2. Reduce `drain_wait_time` in Redis:
   ```bash
   redis-cli HSET gateway:business:config lifecycle.drain_wait_time "300s"
   ```

3. Force delete if necessary:
   ```bash
   kubectl delete pod <pod-name> -n uag --force --grace-period=0
   ```

## Debugging

### Enable debug logging

```bash
export LOG_LEVEL=debug
./uag
```

### Check eBPF programs

```bash
# List loaded programs
bpftool prog list

# Dump maps
bpftool map dump name sock_pair_map
bpftool map dump name ip_blacklist
```

### Network debugging

```bash
# Check listening ports
netstat -tlnp | grep 8080

# Test connectivity
curl -v http://localhost:8080/health

# Monitor connections
ss -tn | grep 8080
```

### Kubernetes debugging

```bash
# Describe pod
kubectl describe pod <pod-name> -n uag

# Check events
kubectl get events -n uag --sort-by='.lastTimestamp'

# Exec into pod
kubectl exec -it <pod-name> -n uag -- sh

# Port forward
kubectl port-forward <pod-name> 8080:8080 -n uag
```

## Performance Tuning

### Increase connection limits

```bash
redis-cli HSET gateway:business:config server.max_connections "50000"
```

### Adjust timeouts

```bash
redis-cli HSET gateway:business:config \
  backends.http.timeout "60s" \
  backends.tcp.timeout "120s"
```

### Resource limits

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 4000m
    memory: 4Gi
```

## Getting Help

1. Check logs: `kubectl logs <pod-name> -n uag`
2. Review metrics: `curl http://localhost:9090/metrics`
3. Check health: `curl http://localhost:9090/health`
4. Open an issue on GitHub with:
   - Gateway version
   - Kubernetes version
   - Error logs
   - Configuration (redacted)

