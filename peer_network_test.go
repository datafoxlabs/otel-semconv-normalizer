// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestPeerNetworkFallback(t *testing.T) {
	tests := []struct {
		name      string
		attrs     map[string]any
		want      map[Concept]string
		flat      string
		nameValue string
	}{
		{
			name:      "redis network only",
			attrs:     map[string]any{"db.system.name": "redis", "network.peer.address": "192.0.2.10", "network.peer.port": int64(6379)},
			want:      map[Concept]string{ConceptPeerDBSystem: "redis", ConceptPeerHostname: "192.0.2.10", ConceptPeerPort: "6379"},
			flat:      "peer.db.system:redis,peer.hostname:192.0.2.10,peer.port:6379",
			nameValue: "192.0.2.10",
		},
		{
			name:      "mysql database name is not a deployment identity",
			attrs:     map[string]any{"db.system.name": "mysql", "db.namespace": "orders", "network.peer.address": "192.0.2.11", "network.peer.port": int64(3306)},
			want:      map[Concept]string{ConceptPeerDBSystem: "mysql", ConceptPeerDBName: "orders", ConceptPeerHostname: "192.0.2.11", ConceptPeerPort: "3306"},
			flat:      "peer.db.name:orders,peer.db.system:mysql,peer.hostname:192.0.2.11,peer.port:3306",
			nameValue: "orders",
		},
		{
			name:      "kafka topic alone also needs endpoint",
			attrs:     map[string]any{"messaging.system": "kafka", "messaging.destination.name": "orders", "network.peer.address": "192.0.2.12", "network.peer.port": int64(9092)},
			want:      map[Concept]string{ConceptPeerMessagingSystem: "kafka", ConceptPeerMessagingDestination: "orders", ConceptPeerHostname: "192.0.2.12", ConceptPeerPort: "9092"},
			flat:      "peer.hostname:192.0.2.12,peer.messaging.destination:orders,peer.messaging.system:kafka,peer.port:9092",
			nameValue: "orders",
		},
		{
			name:      "http network only",
			attrs:     map[string]any{"http.request.method": "GET", "network.peer.address": "192.0.2.13", "network.peer.port": int64(443)},
			want:      map[Concept]string{ConceptPeerHostname: "192.0.2.13", ConceptPeerPort: "443"},
			flat:      "peer.hostname:192.0.2.13,peer.port:443",
			nameValue: "192.0.2.13",
		},
		{
			name:      "rpc name alone is not a deployment identity",
			attrs:     map[string]any{"rpc.system": "grpc", "rpc.service": "orders.v1.Orders", "network.peer.address": "192.0.2.14"},
			want:      map[Concept]string{ConceptPeerRPCSystem: "grpc", ConceptPeerRPCService: "orders.v1.Orders", ConceptPeerHostname: "192.0.2.14"},
			flat:      "peer.hostname:192.0.2.14,peer.rpc.service:orders.v1.Orders,peer.rpc.system:grpc",
			nameValue: "orders.v1.Orders",
		},
		{
			name:      "legacy socket pair",
			attrs:     map[string]any{"net.sock.peer.addr": "192.0.2.15", "net.sock.peer.port": int64(6380)},
			want:      map[Concept]string{ConceptPeerHostname: "192.0.2.15", ConceptPeerPort: "6380"},
			flat:      "peer.hostname:192.0.2.15,peer.port:6380",
			nameValue: "192.0.2.15",
		},
		{
			name:      "legacy IP does not borrow target port",
			attrs:     map[string]any{"net.peer.ip": "192.0.2.16", "net.peer.port": int64(6379)},
			want:      map[Concept]string{ConceptPeerHostname: "192.0.2.16"},
			flat:      "peer.hostname:192.0.2.16",
			nameValue: "192.0.2.16",
		},
		{
			name:      "conflicting socket port is omitted",
			attrs:     map[string]any{"network.peer.address": "192.0.2.17", "net.sock.peer.addr": "192.0.2.18", "net.sock.peer.port": int64(6380)},
			want:      map[Concept]string{ConceptPeerHostname: "192.0.2.17"},
			flat:      "peer.hostname:192.0.2.17",
			nameValue: "192.0.2.17",
		},
		{
			name:      "no endpoint preserves technology-only identity",
			attrs:     map[string]any{"db.system": "redis"},
			want:      map[Concept]string{ConceptPeerDBSystem: "redis"},
			flat:      "peer.db.system:redis",
			nameValue: "redis",
		},
		{
			name:  "port alone does not create an identity",
			attrs: map[string]any{"network.peer.port": int64(6379)},
			want:  map[Concept]string{},
		},
	}
	for _, test := range tests {
		for _, kind := range []ptrace.SpanKind{ptrace.SpanKindClient, ptrace.SpanKindProducer, ptrace.SpanKindConsumer} {
			t.Run(test.name+"/"+kind.String(), func(t *testing.T) {
				attrs := pcommon.NewMap()
				if err := attrs.FromRaw(test.attrs); err != nil {
					t.Fatal(err)
				}
				before := attrs.AsRaw()
				tags := GetPeerTags(attrs, kind)
				got := map[Concept]string{}
				tags.Range(func(key Concept, value string) bool { got[key] = value; return true })
				if !reflect.DeepEqual(got, test.want) {
					t.Fatalf("tags = %v, want %v", got, test.want)
				}
				if got := string(tags.AppendFlattened(nil)); got != test.flat {
					t.Fatalf("flattened = %q, want %q", got, test.flat)
				}
				if got := GetFlattenedPeerTags(attrs, kind); got != test.flat {
					t.Fatalf("compatibility flattened = %q, want %q", got, test.flat)
				}
				if got := GetPeerHostname(attrs, kind); got != test.want[ConceptPeerHostname] {
					t.Fatalf("hostname = %q", got)
				}
				if got := GetPeerPort(attrs, kind); got != test.want[ConceptPeerPort] {
					t.Fatalf("port = %q", got)
				}
				if got := GetPeerName(attrs, kind); got != test.nameValue {
					t.Fatalf("display name = %q, want %q", got, test.nameValue)
				}
				ranged := map[Concept]string{}
				RangePeerAttributes(attrs, kind, func(key Concept, value string) bool { ranged[key] = value; return true })
				if !reflect.DeepEqual(ranged, test.want) {
					t.Fatalf("range = %v, want %v", ranged, test.want)
				}
				if !reflect.DeepEqual(attrs.AsRaw(), before) {
					t.Fatal("read-only peer APIs mutated source")
				}
				SetPeerTags(attrs, kind)
				for key, want := range test.want {
					got, ok := attrs.Get(string(key))
					if !ok || got.Type() != pcommon.ValueTypeStr || got.Str() != want {
						t.Fatalf("persisted %s = %v, want string %q", key, got, want)
					}
				}
				if test.want[ConceptPeerPort] == "" {
					if _, ok := attrs.Get(string(ConceptPeerPort)); ok {
						t.Fatal("unexpected persisted port")
					}
				}
				written := attrs.AsRaw()
				flat, ok := attrs.Get(keyDFFlattenedPeerTags)
				if test.flat == "" && ok {
					t.Fatal("empty projection retained a flattened fingerprint")
				}
				if test.flat != "" && (!ok || flat.Str() != test.flat) {
					t.Fatalf("persisted flattened = %v, want %q", flat, test.flat)
				}
				SetPeerTags(attrs, kind)
				if !reflect.DeepEqual(attrs.AsRaw(), written) {
					t.Fatal("SetPeerTags is not idempotent")
				}
				attrs.Clear()
				if got := string(tags.AppendFlattened(nil)); got != test.flat {
					t.Fatal("snapshot depends on mutated input")
				}
			})
		}
	}
}

func TestPeerNetworkDoesNotChangeLogicalTargets(t *testing.T) {
	fixtures := []struct {
		name  string
		attrs map[string]any
	}{
		{"hostname", map[string]any{"db.system": "redis", "server.address": "cache.internal", "server.port": int64(6379)}},
		{"legacy hostname", map[string]any{"net.peer.name": "cache.internal"}},
		{"explicit service", map[string]any{"peer.service": "shared-cache", "db.system": "redis"}},
		{"cluster list", map[string]any{"server.address": "node2:6380,node1:6379"}},
		{"sentinel list", map[string]any{"server.address": "node2:26379,node1:26379/mymaster"}},
		{"kafka bootstrap", map[string]any{"messaging.system": "kafka", "messaging.destination.name": "orders", "messaging.kafka.bootstrap.servers": "b:9092,a:9092"}},
		{"rocketmq broker", map[string]any{"messaging.system": "rocketmq", "messaging.rocketmq.broker_address": "broker:10911"}},
		{"S3 bucket", map[string]any{"aws.s3.bucket": "orders"}},
		{"DynamoDB table", map[string]any{"db.system": "dynamodb", "aws.dynamodb.table_names": []any{"orders"}}},
		{"Kinesis stream", map[string]any{"messaging.system": "aws_kinesis", "aws.kinesis.stream_name": "orders"}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			if err := attrs.FromRaw(fixture.attrs); err != nil {
				t.Fatal(err)
			}
			before := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient)
			for _, ip := range []string{"192.0.2.20", "192.0.2.21"} {
				attrs.PutStr("network.peer.address", ip)
				attrs.PutInt("network.peer.port", 6381)
				if got := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient); got != before {
					t.Fatalf("identity changed: %q -> %q", before, got)
				}
				if got := GetPeerPort(attrs, ptrace.SpanKindClient); got != "" {
					t.Fatalf("logical target gained network port %q", got)
				}
			}
		})
	}
}

func TestPeerNetworkDistinctEndpoints(t *testing.T) {
	attrs := testAttrs(map[string]string{"db.system": "mysql", "db.namespace": "orders", "network.peer.address": "192.0.2.30"})
	attrs.PutInt("network.peer.port", 3306)
	first := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient)
	attrs.PutStr("network.peer.address", "192.0.2.31")
	second := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient)
	attrs.PutInt("network.peer.port", 3307)
	third := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient)
	if first == second || second == third || first == third {
		t.Fatalf("different endpoints merged: %q, %q, %q", first, second, third)
	}
}

func TestPeerNetworkPortValidity(t *testing.T) {
	for _, test := range []struct {
		name string
		port any
		want string
	}{
		{"minimum", int64(1), "1"}, {"maximum", int64(65535), "65535"},
		{"zero", int64(0), ""}, {"negative", int64(-1), ""}, {"overflow", int64(65536), ""},
		{"string", "6379", ""}, {"float", float64(6379), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			if err := attrs.FromRaw(map[string]any{"network.peer.address": "192.0.2.40", "network.peer.port": test.port}); err != nil {
				t.Fatal(err)
			}
			if got := GetPeerPort(attrs, ptrace.SpanKindClient); got != test.want {
				t.Fatalf("port = %q, want %q", got, test.want)
			}
			SetPeerTags(attrs, ptrace.SpanKindClient)
			value, ok := attrs.Get(string(ConceptPeerPort))
			if test.want == "" && ok {
				t.Fatal("invalid optional port was persisted")
			}
			if test.want != "" && (!ok || value.Str() != test.want) {
				t.Fatal("valid port was not persisted")
			}
		})
	}
}

func TestPeerNetworkKindGate(t *testing.T) {
	for _, kind := range []ptrace.SpanKind{ptrace.SpanKindServer, ptrace.SpanKindInternal, ptrace.SpanKindUnspecified} {
		attrs := testAttrs(map[string]string{"network.peer.address": "192.0.2.50"})
		attrs.PutInt("network.peer.port", 6379)
		before := attrs.AsRaw()
		if got := GetFlattenedPeerTags(attrs, kind); got != "" {
			t.Fatalf("%s emitted %q", kind, got)
		}
		if GetPeerHostname(attrs, kind) != "" || GetPeerPort(attrs, kind) != "" {
			t.Fatalf("%s emitted endpoint identity", kind)
		}
		SetPeerTags(attrs, kind)
		if !reflect.DeepEqual(attrs.AsRaw(), before) {
			t.Fatalf("%s mutated input", kind)
		}
	}
}

func TestSetPeerTagsClearsNetworkPortWhenLogicalTargetAppears(t *testing.T) {
	attrs := testAttrs(map[string]string{"network.peer.address": "192.0.2.60"})
	attrs.PutInt("network.peer.port", 6379)
	SetPeerTags(attrs, ptrace.SpanKindClient)
	attrs.PutStr("server.address", "cache.internal")
	SetPeerTags(attrs, ptrace.SpanKindClient)
	if _, ok := attrs.Get(string(ConceptPeerPort)); ok {
		t.Fatal("stale network port retained beside logical target")
	}
	if got, _ := attrs.Get(string(ConceptPeerHostname)); got.Str() != "cache.internal" {
		t.Fatalf("hostname = %q", got.Str())
	}
	if got, _ := attrs.Get(keyDFFlattenedPeerTags); got.Str() != "peer.hostname:cache.internal" {
		t.Fatalf("flattened = %q", got.Str())
	}
}
