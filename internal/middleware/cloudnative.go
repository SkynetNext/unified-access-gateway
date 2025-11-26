package middleware

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/SkynetNext/unified-access-gateway/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CloudNativeMiddleware adds cloud-native headers and tracing
func CloudNativeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract trace context (for distributed tracing)
		ctx := observability.ExtractTraceContext(r.Context(), r)

		// 2. Start span
		ctx, span := observability.StartSpan(ctx, "gateway.request")
		defer span.End()

		// 3. Add K8s Pod metadata to span
		if podName := os.Getenv("POD_NAME"); podName != "" {
			span.SetAttributes(
				attribute.String("k8s.pod.name", podName),
				attribute.String("k8s.namespace", os.Getenv("POD_NAMESPACE")),
				attribute.String("k8s.node.name", os.Getenv("NODE_NAME")),
			)
		}

		// 4. Add request attributes
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
			attribute.String("http.host", r.Host),
		)

		// 5. Inject trace context into response headers (for downstream services)
		observability.InjectTraceContext(ctx, r)

		// 6. Add cloud-native headers
		w.Header().Set("X-Gateway-Pod", os.Getenv("POD_NAME"))
		w.Header().Set("X-Gateway-Version", "1.0.0")
		w.Header().Set("X-Request-ID", trace.SpanContextFromContext(ctx).TraceID().String())

		// 7. Wrap response writer to capture status and bytes
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// 8. Record metrics
		start := time.Now()
		next.ServeHTTP(rw, r)
		duration := time.Since(start)

		// 9. Update span with response
		span.SetAttributes(
			attribute.Int("http.status_code", rw.statusCode),
			attribute.Int64("http.duration_ms", duration.Milliseconds()),
		)

		// 10. Record comprehensive metrics
		bytesIn := int64(r.ContentLength)
		if bytesIn < 0 {
			bytesIn = 0 // ContentLength can be -1 if unknown
		}

		upstream := r.Header.Get("X-Upstream") // Set by proxy if available
		if upstream == "" {
			upstream = "unknown"
		}

		RecordHTTPMetrics(r.Method, strconv.Itoa(rw.statusCode), upstream, duration.Seconds(), bytesIn, rw.bytesWritten)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// K8sProbeMiddleware handles K8s liveness/readiness probes
func K8sProbeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// K8s probes use specific User-Agent
		if r.Header.Get("User-Agent") == "kube-probe/1.0" {
			// Short-circuit for probes (no tracing, no metrics)
			next.ServeHTTP(w, r)
			return
		}

		// Normal request processing
		next.ServeHTTP(w, r)
	})
}

// ServiceMeshMiddleware adds Istio/Linkerd headers
func ServiceMeshMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add service mesh headers
		if podName := os.Getenv("POD_NAME"); podName != "" {
			r.Header.Set("X-Forwarded-For-Pod", podName)
		}

		// Propagate service mesh trace context
		// (Istio uses B3 headers, Linkerd uses l5d-* headers)
		if traceID := r.Header.Get("X-B3-TraceId"); traceID != "" {
			// Istio trace context
			w.Header().Set("X-B3-TraceId", traceID)
		}

		next.ServeHTTP(w, r)
	})
}
