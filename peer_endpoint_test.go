// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestGetPeerEndpoints(t *testing.T) {
	tests := []struct {
		name  string
		kind  ptrace.SpanKind
		attrs map[string]any
		want  PeerEndpoints
	}{
		{
			name: "lettuce stable database attributes",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"db.system.name":       "redis",
				"server.address":       "redis.example.internal",
				"server.port":          int64(6379),
				"network.peer.address": "192.0.2.10",
				"network.peer.port":    int64(6380),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "redis.example.internal", Port: 6379, HasPort: true},
				Network: PeerEndpoint{Address: "192.0.2.10", Port: 6380, HasPort: true},
			},
		},
		{
			name: "network only is not lost",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"db.system":            "redis",
				"network.peer.address": "192.0.2.11",
				"network.peer.port":    int64(6379),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.11", Port: 6379, HasPort: true,
			}},
		},
		{
			name: "legacy client aliases keep distinct meanings",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"net.peer.name":      "redis.old.internal",
				"net.peer.port":      int64(6379),
				"net.peer.ip":        "192.0.2.12",
				"net.sock.peer.port": int64(6381),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "redis.old.internal", Port: 6379, HasPort: true},
				Network: PeerEndpoint{Address: "192.0.2.12", Port: 6381, HasPort: true},
			},
		},
		{
			name: "legacy socket address",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"net.sock.peer.addr": "192.0.2.13",
				"net.sock.peer.port": int64(6379),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.13", Port: 6379, HasPort: true,
			}},
		},
		{
			name: "different network addresses do not inherit socket port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"net.sock.peer.addr":   "192.0.2.20",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{Address: "192.0.2.10"}},
		},
		{
			name: "different target addresses do not inherit legacy port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address": "mysql.prod.internal",
				"net.peer.name":  "mysql.old.internal",
				"net.peer.port":  int64(3307),
			},
			want: PeerEndpoints{Target: PeerEndpoint{Address: "mysql.prod.internal"}},
		},
		{
			name: "matching target addresses allow legacy port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address": "mysql.prod.internal",
				"net.peer.name":  "mysql.prod.internal",
				"net.peer.port":  int64(3306),
			},
			want: PeerEndpoints{Target: PeerEndpoint{
				Address: "mysql.prod.internal", Port: 3306, HasPort: true,
			}},
		},
		{
			name: "matching network addresses allow socket port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"net.sock.peer.addr":   "192.0.2.10",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.10", Port: 6380, HasPort: true,
			}},
		},
		{
			name: "no competing target address allows partial legacy port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address": "mysql.prod.internal",
				"net.peer.port":  int64(3306),
			},
			want: PeerEndpoints{Target: PeerEndpoint{
				Address: "mysql.prod.internal", Port: 3306, HasPort: true,
			}},
		},
		{
			name: "conflicting legacy peer IP without socket address blocks socket port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"net.peer.ip":          "192.0.2.20",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{Address: "192.0.2.10"}},
		},
		{
			name: "matching socket address wins over unrelated legacy peer IP",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"net.sock.peer.addr":   "192.0.2.10",
				"net.peer.ip":          "192.0.2.20",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.10", Port: 6380, HasPort: true,
			}},
		},
		{
			name: "canonical network port is used despite conflicting legacy address",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"network.peer.port":    int64(6379),
				"net.sock.peer.addr":   "192.0.2.20",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.10", Port: 6379, HasPort: true,
			}},
		},
		{
			name: "partial canonical network address accepts legacy socket port without conflict",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.10",
				"net.sock.peer.port":   int64(6380),
			},
			want: PeerEndpoints{Network: PeerEndpoint{
				Address: "192.0.2.10", Port: 6380, HasPort: true,
			}},
		},
		{
			name: "canonical fields outrank legacy",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address":       "new-target",
				"net.peer.name":        "old-target",
				"server.port":          int64(6380),
				"net.peer.port":        int64(6379),
				"network.peer.address": "192.0.2.14",
				"net.sock.peer.addr":   "192.0.2.15",
				"net.peer.ip":          "192.0.2.16",
				"network.peer.port":    int64(6382),
				"net.sock.peer.port":   int64(6383),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "new-target", Port: 6380, HasPort: true},
				Network: PeerEndpoint{Address: "192.0.2.14", Port: 6382, HasPort: true},
			},
		},
		{
			name: "invalid values fall back to compatible aliases",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address":       int64(12),
				"net.peer.name":        "backup-target",
				"server.port":          "not-an-integer",
				"net.peer.port":        int64(6379),
				"network.peer.address": int64(13),
				"net.sock.peer.addr":   "192.0.2.17",
				"network.peer.port":    int64(65536),
				"net.sock.peer.port":   int64(6381),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "backup-target", Port: 6379, HasPort: true},
				Network: PeerEndpoint{Address: "192.0.2.17", Port: 6381, HasPort: true},
			},
		},
		{
			name: "do not pair network address with legacy target port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"net.peer.ip":   "192.0.2.18",
				"net.peer.port": int64(6379),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Port: 6379, HasPort: true},
				Network: PeerEndpoint{Address: "192.0.2.18"},
			},
		},
		{
			name: "preserve cluster composite target verbatim",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address":       "node1:6379,node2:6380",
				"network.peer.address": "192.0.2.19",
				"network.peer.port":    int64(6380),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "node1:6379,node2:6380"},
				Network: PeerEndpoint{Address: "192.0.2.19", Port: 6380, HasPort: true},
			},
		},
		{
			name: "preserve sentinel target verbatim",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address": "node1:26379,node2:26379/mymaster",
			},
			want: PeerEndpoints{Target: PeerEndpoint{
				Address: "node1:26379,node2:26379/mymaster",
			}},
		},
		{
			name: "preserve ipv6 address and distinct port",
			kind: ptrace.SpanKindClient,
			attrs: map[string]any{
				"server.address":       "2001:db8::1",
				"server.port":          int64(6379),
				"network.peer.address": "2001:db8::2",
				"network.peer.port":    int64(6380),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Address: "2001:db8::1", Port: 6379, HasPort: true},
				Network: PeerEndpoint{Address: "2001:db8::2", Port: 6380, HasPort: true},
			},
		},
		{
			name: "port-only spans remain partial",
			kind: ptrace.SpanKindProducer,
			attrs: map[string]any{
				"server.port":       int64(6380),
				"network.peer.port": int64(6381),
			},
			want: PeerEndpoints{
				Target:  PeerEndpoint{Port: 6380, HasPort: true},
				Network: PeerEndpoint{Port: 6381, HasPort: true},
			},
		},
		{
			name: "consumer spans supported",
			kind: ptrace.SpanKindConsumer,
			attrs: map[string]any{
				"server.address": "broker.internal",
			},
			want: PeerEndpoints{Target: PeerEndpoint{Address: "broker.internal"}},
		},
		{
			name: "internal spans ignored",
			kind: ptrace.SpanKindInternal,
			attrs: map[string]any{
				"server.address": "redis.internal",
				"server.port":    int64(6379),
			},
		},
		{
			name: "server spans ignored",
			kind: ptrace.SpanKindServer,
			attrs: map[string]any{
				"network.peer.address": "192.0.2.20",
			},
		},
		{
			name: "unspecified spans ignored",
			kind: ptrace.SpanKindUnspecified,
			attrs: map[string]any{
				"server.address": "redis.internal",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for key, raw := range test.attrs {
				switch value := raw.(type) {
				case string:
					attrs.PutStr(key, value)
				case int64:
					attrs.PutInt(key, value)
				default:
					t.Fatalf("unsupported fixture value %T", raw)
				}
			}
			if got := GetPeerEndpoints(attrs, test.kind); got != test.want {
				t.Fatalf("GetPeerEndpoints() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestGetPeerEndpointsDoesNotMutateAndKeepsExistingPeerTags(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("db.system.name", "redis")
	attrs.PutStr("server.address", "redis.example.internal")
	attrs.PutInt("server.port", 6379)
	attrs.PutStr("network.peer.address", "192.0.2.25")
	attrs.PutInt("network.peer.port", 6379)

	before := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient)
	count := attrs.Len()
	got := GetPeerEndpoints(attrs, ptrace.SpanKindClient)
	if got.Target.Address != "redis.example.internal" || got.Network.Address != "192.0.2.25" {
		t.Fatalf("unexpected endpoints: %+v", got)
	}
	if attrs.Len() != count {
		t.Fatalf("attributes mutated: len = %d, want %d", attrs.Len(), count)
	}
	if after := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient); after != before {
		t.Fatalf("flattened peer tags changed: %q -> %q", before, after)
	}
}

var peerEndpointsBenchmarkSink PeerEndpoints

func TestGetPeerEndpointsZeroAlloc(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("server.address", "redis.example.internal")
	attrs.PutInt("server.port", 6379)
	attrs.PutStr("network.peer.address", "192.0.2.26")
	attrs.PutInt("network.peer.port", 6379)
	allocs := testing.AllocsPerRun(100, func() {
		peerEndpointsBenchmarkSink = GetPeerEndpoints(attrs, ptrace.SpanKindClient)
	})
	if allocs != 0 {
		t.Fatalf("GetPeerEndpoints allocated %v times, want 0", allocs)
	}
}

func BenchmarkGetPeerEndpoints(b *testing.B) {
	attrs := pcommon.NewMap()
	attrs.PutStr("server.address", "redis.example.internal")
	attrs.PutInt("server.port", 6379)
	attrs.PutStr("network.peer.address", "192.0.2.26")
	attrs.PutInt("network.peer.port", 6379)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peerEndpointsBenchmarkSink = GetPeerEndpoints(attrs, ptrace.SpanKindClient)
	}
}
