package discovery

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// K8sServiceDiscovery provides Kubernetes-native service discovery
type K8sServiceDiscovery struct {
	namespace string
}

// NewK8sServiceDiscovery creates a new K8s service discovery
func NewK8sServiceDiscovery() *K8sServiceDiscovery {
	// Get namespace from Pod metadata (injected by K8s)
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		// Fallback: read from /var/run/secrets/kubernetes.io/serviceaccount/namespace
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			namespace = strings.TrimSpace(string(data))
		} else {
			namespace = "default"
		}
	}

	return &K8sServiceDiscovery{
		namespace: namespace,
	}
}

// ResolveService resolves a K8s service name to address
// Format: <service-name> or <service-name>.<namespace>.svc.cluster.local
func (k *K8sServiceDiscovery) ResolveService(serviceName string) (string, error) {
	// If already FQDN, use as-is
	if strings.Contains(serviceName, ".") {
		return serviceName, nil
	}

	// Build FQDN: <service>.<namespace>.svc.cluster.local
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, k.namespace)

	// Resolve DNS (K8s CoreDNS)
	ips, err := net.LookupIP(fqdn)
	if err != nil {
		// Fallback to short name (same namespace)
		ips, err = net.LookupIP(serviceName)
		if err != nil {
			return "", fmt.Errorf("failed to resolve service %s: %w", serviceName, err)
		}
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IPs found for service %s", serviceName)
	}

	// Return first IP (or use SRV records for port)
	return ips[0].String(), nil
}

// ResolveServiceWithPort resolves service and returns address:port
func (k *K8sServiceDiscovery) ResolveServiceWithPort(serviceName string, port int) (string, error) {
	ip, err := k.ResolveService(serviceName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", ip, port), nil
}

// ResolveServiceDNS returns the FQDN for a service
func (k *K8sServiceDiscovery) ResolveServiceDNS(serviceName string) string {
	if strings.Contains(serviceName, ".") {
		return serviceName
	}
	return fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, k.namespace)
}

// GetPodName returns the current Pod name (from K8s downward API)
func GetPodName() string {
	return os.Getenv("POD_NAME")
}

// GetNodeName returns the current Node name
func GetNodeName() string {
	return os.Getenv("NODE_NAME")
}

// IsRunningInK8s checks if running in Kubernetes
func IsRunningInK8s() bool {
	// Check for K8s service account token
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	return err == nil
}

// WatchServiceEndpoints watches for service endpoint changes (future enhancement)
func (k *K8sServiceDiscovery) WatchServiceEndpoints(serviceName string, callback func([]string)) {
	// This would use K8s API client to watch Endpoints
	// For now, just periodic DNS lookup
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			ips, err := net.LookupIP(k.ResolveServiceDNS(serviceName))
			if err == nil {
				var addrs []string
				for _, ip := range ips {
					addrs = append(addrs, ip.String())
				}
				callback(addrs)
			}
		}
	}()
}

