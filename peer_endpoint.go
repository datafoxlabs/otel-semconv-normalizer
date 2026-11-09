// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// PeerEndpoint is one observed endpoint. Address is preserved verbatim; it can
// be a hostname, IP, Unix socket path, or a SemConv database target containing
// multiple configured endpoints. Port is only meaningful when HasPort is true.
// Ports never cross target/network boundaries; legacy port fallbacks are
// suppressed when the corresponding addresses explicitly disagree.
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

	targetAddress := peerEndpointString(attrs, "server.address")
	legacyTargetAddress := peerEndpointString(attrs, "net.peer.name")
	endpoints.Target.Address = targetAddress
	if targetAddress == "" {
		endpoints.Target.Address = legacyTargetAddress
	}
	endpoints.Target.Port, endpoints.Target.HasPort = peerEndpointPort(attrs, "server.port")
	// A legacy port may describe the legacy address rather than the selected
	// canonical one. Preserve the selected address but do not invent a pair.
	if !endpoints.Target.HasPort && !peerEndpointAddressConflict(targetAddress, legacyTargetAddress) {
		endpoints.Target.Port, endpoints.Target.HasPort = peerEndpointPort(attrs, "net.peer.port")
	}

	networkAddress := peerEndpointString(attrs, "network.peer.address")
	socketAddress := peerEndpointString(attrs, "net.sock.peer.addr")
	legacyPeerIP := peerEndpointString(attrs, "net.peer.ip")
	endpoints.Network.Address = networkAddress
	if endpoints.Network.Address == "" {
		endpoints.Network.Address = socketAddress
	}
	if endpoints.Network.Address == "" {
		endpoints.Network.Address = legacyPeerIP
	}
	endpoints.Network.Port, endpoints.Network.HasPort = peerEndpointPort(attrs, "network.peer.port")
	// net.sock.peer.port belongs to the legacy socket endpoint. If the
	// canonical network address disagrees with the legacy socket address,
	// using its port would fabricate an endpoint. With no socket address,
	// also reject a fallback when net.peer.ip explicitly disagrees.
	if !endpoints.Network.HasPort &&
		!peerEndpointAddressConflict(networkAddress, socketAddress) &&
		(socketAddress != "" || !peerEndpointAddressConflict(networkAddress, legacyPeerIP)) {
		endpoints.Network.Port, endpoints.Network.HasPort = peerEndpointPort(attrs, "net.sock.peer.port")
	}

	return endpoints
}

// peerEndpointAddressConflict only rejects a proven mismatch. Missing or
// invalid addresses are not proof of a different endpoint; partial inputs may
// still use a legacy port when no conflicting address is present.
func peerEndpointAddressConflict(selected, alternate string) bool {
	return selected != "" && alternate != "" && selected != alternate
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
