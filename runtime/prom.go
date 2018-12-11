package runtime

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
)

// MetricsIntercept ...
type MetricsIntercept struct {
	monitoring *monitoring
	trackPeers bool
}

// NewMetricsIntercept implements both prometheus Collector interface and methods required by grpc MetricsIntercept.
func NewMetricsIntercept() *MetricsIntercept {
	return &MetricsIntercept{
		monitoring: initMonitoring(true),
		trackPeers: true,
	}
}

func RegisterMetricsIntercept(s *grpc.Server, i *MetricsIntercept) (err error) {
	if i.trackPeers {
		return nil
	}

	infos := s.GetServiceInfo()
	for sn, info := range infos {
		for _, m := range info.Methods {
			t := handlerType(m.IsClientStream, m.IsServerStream)

			for c := uint32(0); c <= 15; c++ {
				requestLabels := prometheus.Labels{
					"service": sn,
					"handler": m.Name,
					"code":    codes.Code(c).String(),
					"type":    t,
				}
				messageLabels := prometheus.Labels{
					"service": sn,
					"handler": m.Name,
				}

				// server
				if _, err = i.monitoring.server.errors.GetMetricWith(requestLabels); err != nil {
					return err
				}
				if _, err = i.monitoring.server.requestsTotal.GetMetricWith(requestLabels); err != nil {
					return err
				}
				if _, err = i.monitoring.server.requestDuration.GetMetricWith(requestLabels); err != nil {
					return err
				}
				if m.IsClientStream {
					if _, err = i.monitoring.server.messagesReceived.GetMetricWith(messageLabels); err != nil {
						return err
					}
				}
				if m.IsServerStream {
					if _, err = i.monitoring.server.messagesSend.GetMetricWith(messageLabels); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Dialer ...
func (i *MetricsIntercept) Dialer(f func(string, time.Duration) (net.Conn, error)) func(string, time.Duration) (net.Conn, error) {
	return func(addr string, timeout time.Duration) (net.Conn, error) {
		i.monitoring.dialer.WithLabelValues(addr).Inc()
		return f(addr, timeout)
	}
}

// UnaryClient ...
func (i *MetricsIntercept) UnaryClient() grpc.UnaryClientInterceptor {
	monitor := i.monitoring.client

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()

		err := invoker(ctx, method, req, reply, cc, opts...)
		code := grpc.Code(err)
		service, method := split(method)
		labels := prometheus.Labels{
			"service": service,
			"handler": method,
			"code":    code.String(),
			"type":    "unary",
		}
		if err != nil && code != codes.OK {
			monitor.errors.With(labels).Add(1)
		}

		monitor.requestDuration.With(labels).Observe(time.Since(start).Seconds())
		monitor.requestsTotal.With(labels).Add(1)

		return err
	}
}

// StreamClient ...
func (i *MetricsIntercept) StreamClient() grpc.StreamClientInterceptor {
	monitor := i.monitoring.client

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()

		client, err := streamer(ctx, desc, cc, method, opts...)
		code := grpc.Code(err)
		service, method := split(method)
		labels := prometheus.Labels{
			"service": service,
			"handler": method,
			"code":    code.String(),
			"type":    handlerType(desc.ClientStreams, desc.ServerStreams),
		}
		if err != nil && code != codes.OK {
			monitor.errors.With(labels).Add(1)
		}

		monitor.requestDuration.With(labels).Observe(time.Since(start).Seconds())
		monitor.requestsTotal.With(labels).Add(1)

		return &monitoredClientStream{ClientStream: client, monitor: monitor, labels: prometheus.Labels{
			"service": service,
			"handler": method,
		}}, err
	}
}

// UnaryServer ...
func (i *MetricsIntercept) UnaryServer() grpc.UnaryServerInterceptor {
	monitor := i.monitoring.server

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		res, err := handler(ctx, req)
		code := grpc.Code(err)
		service, method := split(info.FullMethod)

		labels := prometheus.Labels{
			"service": service,
			"handler": method,
			"code":    code.String(),
			"type":    "unary",
		}
		if i.trackPeers {
			labels["peer"] = peerValue(ctx)
		}
		if err != nil && code != codes.OK {
			monitor.errors.With(labels).Add(1)
		}

		monitor.requestDuration.With(labels).Observe(time.Since(start).Seconds())
		monitor.requestsTotal.With(labels).Add(1)

		return res, err
	}
}

// StreamServer ...
func (i *MetricsIntercept) StreamServer() grpc.StreamServerInterceptor {
	monitor := i.monitoring.server

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		service, method := split(info.FullMethod)
		streamLabels := prometheus.Labels{
			"service": service,
			"handler": method,
		}
		if i.trackPeers {
			if ss != nil {
				streamLabels["peer"] = peerValue(ss.Context())
			} else {
				// mostly for testing purposes
				streamLabels["peer"] = "nil-server-stream"
			}
		}
		err := handler(srv, &monitoredServerStream{ServerStream: ss, labels: streamLabels, monitor: monitor})
		code := grpc.Code(err)
		labels := prometheus.Labels{
			"service": service,
			"handler": method,
			"code":    code.String(),
			"type":    handlerType(info.IsClientStream, info.IsServerStream),
		}
		if i.trackPeers {
			if ss != nil {
				labels["peer"] = peerValue(ss.Context())
			} else {
				// mostly for testing purposes
				labels["peer"] = "nil-server-stream"
			}
		}
		if err != nil && code != codes.OK {
			monitor.errors.With(labels).Add(1)
		}

		monitor.requestDuration.With(labels).Observe(time.Since(start).Seconds())
		monitor.requestsTotal.With(labels).Add(1)

		return err
	}
}

// Describe implements prometheus Collector interface.
func (i *MetricsIntercept) Describe(in chan<- *prometheus.Desc) {
	i.monitoring.dialer.Describe(in)
	i.monitoring.server.Describe(in)
	i.monitoring.client.Describe(in)
}

// Collect implements prometheus Collector interface.
func (i *MetricsIntercept) Collect(in chan<- prometheus.Metric) {
	i.monitoring.dialer.Collect(in)
	i.monitoring.server.Collect(in)
	i.monitoring.client.Collect(in)
}

type ctxKey int

var (
	tagRPCKey  ctxKey = 1
	tagConnKey ctxKey = 2
)

// TagRPC implements stats Handler interface.
func (i *MetricsIntercept) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	service, method := split(info.FullMethodName)

	return context.WithValue(ctx, tagRPCKey, prometheus.Labels{
		"fail_fast": strconv.FormatBool(info.FailFast),
		"service":   service,
		"handler":   method,
	})
}

// HandleRPC implements stats Handler interface.
func (i *MetricsIntercept) HandleRPC(ctx context.Context, stat stats.RPCStats) {
	lab, _ := ctx.Value(tagRPCKey).(prometheus.Labels)

	switch in := stat.(type) {
	case *stats.Begin:
		if in.IsClient() {
			i.monitoring.client.requests.With(lab).Inc()
		} else {
			i.monitoring.server.requests.With(lab).Inc()
		}
	case *stats.End:
		if in.IsClient() {
			i.monitoring.client.requests.With(lab).Dec()
		} else {
			i.monitoring.server.requests.With(lab).Dec()
		}
	}
}

// TagConn implements stats Handler interface.
func (i *MetricsIntercept) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, tagConnKey, prometheus.Labels{
		"remote_addr": info.RemoteAddr.String(),
		"local_addr":  info.LocalAddr.String(),
	})
}

// HandleConn implements stats Handler interface.
func (i *MetricsIntercept) HandleConn(ctx context.Context, stat stats.ConnStats) {
	lab, _ := ctx.Value(tagConnKey).(prometheus.Labels)

	switch in := stat.(type) {
	case *stats.ConnBegin:
		if in.IsClient() {
			i.monitoring.client.connections.With(lab).Inc()
		} else {
			i.monitoring.server.connections.With(lab).Inc()
		}
	case *stats.ConnEnd:
		if in.IsClient() {
			i.monitoring.client.connections.With(lab).Dec()
		} else {
			i.monitoring.server.connections.With(lab).Dec()
		}
	}
}

type monitoring struct {
	dialer *prometheus.CounterVec
	server *monitor
	client *monitor
}

type monitor struct {
	connections      *prometheus.GaugeVec
	requests         *prometheus.GaugeVec
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	messagesReceived *prometheus.CounterVec
	messagesSend     *prometheus.CounterVec
	errors           *prometheus.CounterVec
}

// Describe implements prometheus Collector interface.
func (m *monitor) Describe(in chan<- *prometheus.Desc) {
	// Gauge
	m.connections.Describe(in)
	m.requests.Describe(in)

	// HistogramVec
	m.requestDuration.Describe(in)

	// CounterVec
	m.requestsTotal.Describe(in)
	m.messagesReceived.Describe(in)
	m.messagesSend.Describe(in)
	m.errors.Describe(in)
}

// Collect implements prometheus Collector interface.
func (m *monitor) Collect(in chan<- prometheus.Metric) {
	// Gauge
	m.connections.Collect(in)
	m.requests.Collect(in)

	// HistogramVec
	m.requestDuration.Collect(in)

	// CounterVec
	m.requestsTotal.Collect(in)
	m.messagesReceived.Collect(in)
	m.messagesSend.Collect(in)
	m.errors.Collect(in)
}

func initMonitoring(trackPeers bool) *monitoring {
	dialer := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "reconnects_total",
			Help:      "Total number of reconnects made by client.",
		},
		[]string{"address"},
	)

	serverConnections := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "connections",
			Help:      "Number of currently opened server side connections.",
		},
		[]string{"remote_addr", "local_addr"},
	)
	serverRequests := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "requests",
			Help:      "Number of currently processed server side rpc requests.",
		},
		[]string{"fail_fast", "handler", "service"},
	)
	serverRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "requests_total",
			Help:      "Total number of RPC requests received by server.",
		},
		appendIf(trackPeers, []string{"service", "handler", "code", "type"}, "peer"),
	)
	serverReceivedMessages := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "received_messages_total",
			Help:      "Total number of RPC messages received by server.",
		},
		appendIf(trackPeers, []string{"service", "handler"}, "peer"),
	)
	serverSendMessages := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "send_messages_total",
			Help:      "Total number of RPC messages send by server.",
		},
		appendIf(trackPeers, []string{"service", "handler"}, "peer"),
	)
	serverRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "request_duration_seconds",
			Help:      "The RPC request latencies in seconds on server side.",
		},
		appendIf(trackPeers, []string{"service", "handler", "code", "type"}, "peer"),
	)
	serverErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "server",
			Name:      "errors_total",
			Help:      "Total number of errors that happen during RPC calles on server side.",
		},
		appendIf(trackPeers, []string{"service", "handler", "code", "type"}, "peer"),
	)

	clientConnections := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "connections",
			Help:      "Number of currently opened client side connections.",
		},
		[]string{"remote_addr", "local_addr"},
	)
	clientRequests := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "requests",
			Help:      "Number of currently processed client side rpc requests.",
		},
		[]string{"fail_fast", "handler", "service"},
	)
	clientRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "requests_total",
			Help:      "Total number of RPC requests made by client.",
		},
		[]string{"service", "handler", "code", "type"},
	)
	clientReceivedMessages := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "received_messages_total",
			Help:      "Total number of RPC messages received.",
		},
		[]string{"service", "handler"},
	)
	clientSendMessages := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "send_messages_total",
			Help:      "Total number of RPC messages send.",
		},
		[]string{"service", "handler"},
	)
	clientRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "request_duration_seconds",
			Help:      "The RPC request latencies in seconds on client side.",
		},
		[]string{"service", "handler", "code", "type"},
	)
	clientErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grpc",
			Subsystem: "client",
			Name:      "errors_total",
			Help:      "Total number of errors that happen during RPC calls.",
		},
		[]string{"service", "handler", "code", "type"},
	)

	return &monitoring{
		dialer: dialer,
		server: &monitor{
			connections:      serverConnections,
			requests:         serverRequests,
			requestsTotal:    serverRequestsTotal,
			requestDuration:  serverRequestDuration,
			messagesReceived: serverReceivedMessages,
			messagesSend:     serverSendMessages,
			errors:           serverErrors,
		},
		client: &monitor{
			connections:      clientConnections,
			requests:         clientRequests,
			requestsTotal:    clientRequestsTotal,
			requestDuration:  clientRequestDuration,
			messagesReceived: clientReceivedMessages,
			messagesSend:     clientSendMessages,
			errors:           clientErrors,
		},
	}
}

type monitoredServerStream struct {
	grpc.ServerStream
	labels  prometheus.Labels
	monitor *monitor
}

func (mss *monitoredServerStream) SendMsg(m interface{}) error {
	err := mss.ServerStream.SendMsg(m)
	if err == nil {
		mss.monitor.messagesSend.With(mss.labels).Inc()
	}
	return err
}

func (mss *monitoredServerStream) RecvMsg(m interface{}) error {
	err := mss.ServerStream.RecvMsg(m)
	if err == nil {
		mss.monitor.messagesReceived.With(mss.labels).Inc()
	}
	return err
}

type monitoredClientStream struct {
	grpc.ClientStream
	labels  prometheus.Labels
	monitor *monitor
}

func (mcs *monitoredClientStream) SendMsg(m interface{}) error {
	err := mcs.ClientStream.SendMsg(m)
	if err == nil {
		mcs.monitor.messagesSend.With(mcs.labels).Inc()
	}
	return err
}

func (mcs *monitoredClientStream) RecvMsg(m interface{}) error {
	err := mcs.ClientStream.RecvMsg(m)
	if err == nil {
		mcs.monitor.messagesReceived.With(mcs.labels).Inc()
	}
	return err
}

func handlerType(clientStream, serverStream bool) string {
	switch {
	case !clientStream && !serverStream:
		return "unary"
	case !clientStream && serverStream:
		return "server_stream"
	case clientStream && !serverStream:
		return "client_stream"
	default:
		return "bidirectional_stream"
	}
}

func split(name string) (string, string) {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[1:i], name[i+1:]
	}
	return "unknown", "unknown"
}

func peerValue(ctx context.Context) string {
	v, ok := peer.FromContext(ctx)
	if !ok {
		return "none"
	}
	return v.Addr.String()
}

func appendIf(ok bool, arr []string, val string) []string {
	if !ok {
		return arr
	}
	return append(arr, val)
}
