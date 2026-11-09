// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// PeerEndpoint is one observed endpoint. Address is preserved verbatim; it can
// be a hostname, IP, Unix socket path, or a SemConv database target containing
// multiple configured endpoints. Port is only meaningful when HasPort is true.
// An address and port are not synthesized by mixing target and network fields.
type PeerEndpoint struct {
	Address string
	Port    uint16
	HasPort bool
}

// PeerEndpoints separates the configured/logical server target from the
// network peer contacted by an outbound span. Either endpoint may be partial.
type PeerEndpoints struct {
	Target  PeerEndpoint
	Network PeerEndpoint
}

// GetPeerEndpoints extracts both endpoints without mutating span attributes.
// It does not infer a deployment identity, resolve DNS, split compound database
// targets, or change the existing GetPeerTags/flattened fingerprint contract.
//
// Source aliases follow the OpenTelemetry network attribute migration:
//   - target address: server.address -> net.peer.name
//   - target port:    server.port -> net.peer.port
//   - network address: network.peer.address -> net.sock.peer.addr -> net.peer.ip
//   - network port:    network.peer.port -> net.sock.peer.port
//
// net.peer.port is a legacy *server* port on client spans, not a network peer
// port. Never combine it with network.peer.address to invent a socket endpoint.
// Only CLIENT, PRODUCER and CONSUMER spans may return endpoint information.
func GetPeerEndpoints(attrs pcommon.Map, kind ptrace.SpanKind) PeerEndpoints {
	if attrs.Len() == 0 || !isPeerSpanKind(kind) {
		return PeerEndpoints{}
	}

	var endpoints PeerEndpoints

	endpoints.Target.Address = peerEndpointString(attrs, "server.address")
	if endpoints.Target.Address == "" {
		endpoints.Target.Address = peerEndpointString(attrs, "net.peer.name")
	}
	endpoints.Target.Port, endpoints.Target.HasPort = peerEndpointPort(attrs, "server.port")
	if !endpoints.Target.HasPort {
		endpoints.Target.Port, endpoints.Target.HasPort = peerEndpointPort(attrs, "net.peer.port")
	}

	endpoints.Network.Address = peerEndpointString(attrs, "network.peer.address")
	if endpoints.Network.Address == "" {
		endpoints.Network.Address = peerEndpointString(attrs, "net.sock.peer.addr")
	}
	if endpoints.Network.Address == "" {
		endpoints.Network.Address = peerEndpointString(attrs, "net.peer.ip")
	}
	endpoints.Network.Port, endpoints.Network.HasPort = peerEndpointPort(attrs, "network.peer.port")
	if !endpoints.Network.HasPort {
		endpoints.Network.Port, endpoints.Network.HasPort = peerEndpointPort(attrs, "net.sock.peer.port")
	}

	return endpoints
}

func peerEndpointString(attrs pcommon.Map, key string) string {
	value, ok := attrs.Get(key)
	if !ok || value.Type() != pcommon.ValueTypeStr {
		return ""
	}
	return value.Str()
}

func peerEndpointPort(attrs pcommon.Map, key string) (uint16, bool) {
	value, ok := attrs.Get(key)
	if !ok || value.Type() != pcommon.ValueTypeInt {
		return 0, false
	}
	port := value.Int()
	if port < 1 || port > 65535 {
		return 0, false
	}
	return uint16(port), true
}
