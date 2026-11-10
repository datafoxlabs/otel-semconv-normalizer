// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type peerTagGolden struct {
	concept Concept
	value   string
}

var peerTagsAppendSink []byte

func TestPeerTagsExactGolden(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr(keyPeerService, "checkout-service")
	attrs.PutStr(keyNetPeerName, "legacy.example")
	attrs.PutStr(keyServerAddress, "current.example")
	attrs.PutStr(keyDBName, "legacy-db")
	attrs.PutStr(keyDBNamespace, "current-db")
	attrs.PutStr(keyMessagingDestination, "legacy-topic")
	attrs.PutStr(keyMessagingDestinationName, "current-topic")
	attrs.PutStr(keyRPCService, "orders.v1.OrderService")
	tables := attrs.PutEmptySlice(keyAWSDynamoDBTableNames)
	tables.AppendEmpty().SetStr("")
	tables.AppendEmpty().SetStr("orders-table")
	tables.AppendEmpty().SetStr("ignored-table")
	attrs.PutStr(keyAWSS3Bucket, "invoices")
	attrs.PutStr(keyAWSKinesisStreamName, "order-events")
	attrs.PutStr(keyDBType, "oracle")
	attrs.PutStr(keyDBSystem, "mysql")
	attrs.PutStr(keyDBSystemName, "mssql")
	attrs.PutStr(keyMessagingSystem, "kafka")
	attrs.PutStr(keyRPCSystem, "grpc")
	attrs.PutStr(keyRPCSystemName, "connect_rpc")
	attrs.PutStr(keyMessagingKafkaBootstrapServers, "kafka:9092")
	attrs.PutStr(keyMessagingRocketMQBrokerAddress, "rocket:10911")

	tags := GetPeerTags(attrs, ptrace.SpanKindClient)
	var got []peerTagGolden
	tags.Range(func(concept Concept, value string) bool {
		got = append(got, peerTagGolden{concept: concept, value: value})
		return true
	})
	want := []peerTagGolden{
		{ConceptPeerService, "checkout-service"},
		{ConceptPeerHostname, "current.example"},
		{ConceptPeerDBName, "current-db"},
		{ConceptPeerMessagingDestination, "current-topic"},
		{ConceptPeerRPCService, "orders.v1.OrderService"},
		{ConceptPeerAWSDynamoDBTable, "orders-table"},
		{ConceptPeerAWSS3Bucket, "invoices"},
		{ConceptPeerAWSKinesisStream, "order-events"},
		{ConceptPeerDBSystem, "microsoft.sql_server"},
		{ConceptPeerMessagingSystem, "kafka"},
		{ConceptPeerRPCSystem, "connectrpc"},
		{ConceptPeerKafkaBootstrapSrvs, "kafka:9092"},
		{ConceptPeerRocketMQBroker, "rocket:10911"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Range golden = %#v, want %#v", got, want)
	}

	wantFlattened := "peer.aws.dynamodb.table:orders-table," +
		"peer.aws.kinesis.stream:order-events," +
		"peer.aws.s3.bucket:invoices," +
		"peer.db.name:current-db," +
		"peer.db.system:microsoft.sql_server," +
		"peer.hostname:current.example," +
		"peer.kafka.bootstrap.servers:kafka:9092," +
		"peer.messaging.destination:current-topic," +
		"peer.messaging.system:kafka," +
		"peer.rocketmq.broker:rocket:10911," +
		"peer.rpc.service:orders.v1.OrderService," +
		"peer.rpc.system:connectrpc," +
		"peer.service:checkout-service"
	if gotFlattened := string(tags.AppendFlattened(nil)); gotFlattened != wantFlattened {
		t.Fatalf("AppendFlattened golden = %q, want %q", gotFlattened, wantFlattened)
	}
}

func TestPeerTagsAppendFlattenedCapacityBoundaries(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyDBSystem:      "redis",
		keyServerAddress: "localhost",
	})
	tags := GetPeerTags(attrs, ptrace.SpanKindClient)
	want := "peer.db.system:redis,peer.hostname:localhost"

	t.Run("exact capacity", func(t *testing.T) {
		dst := make([]byte, 0, len(want))
		allocs := testing.AllocsPerRun(1000, func() {
			dst = tags.AppendFlattened(dst[:0])
			peerTagsAppendSink = dst
		})
		if allocs != 0 {
			t.Fatalf("exact-capacity AppendFlattened allocated %v times, want 0", allocs)
		}
		if string(dst) != want || cap(dst) != len(want) {
			t.Fatalf("exact-capacity output = %q cap=%d, want %q cap=%d", dst, cap(dst), want, len(want))
		}
	})

	t.Run("prefix with exact remaining capacity", func(t *testing.T) {
		const prefix = "prefix/"
		dst := make([]byte, len(prefix), len(prefix)+len(want))
		copy(dst, prefix)
		allocs := testing.AllocsPerRun(1000, func() {
			dst = tags.AppendFlattened(dst[:len(prefix)])
			peerTagsAppendSink = dst
		})
		if allocs != 0 {
			t.Fatalf("prefix exact-capacity AppendFlattened allocated %v times, want 0", allocs)
		}
		if got, expected := string(dst), prefix+want; got != expected {
			t.Fatalf("prefix exact-capacity output = %q, want %q", got, expected)
		}
	})

	t.Run("one byte short grows", func(t *testing.T) {
		dst := make([]byte, 0, len(want)-1)
		oldCapacity := cap(dst)
		dst = tags.AppendFlattened(dst)
		if string(dst) != want {
			t.Fatalf("one-byte-short output = %q, want %q", dst, want)
		}
		if cap(dst) <= oldCapacity {
			t.Fatalf("one-byte-short capacity did not grow: before=%d after=%d", oldCapacity, cap(dst))
		}
	})

	t.Run("long value exact capacity", func(t *testing.T) {
		longValue := strings.Repeat("x", 16*1024)
		longTags := GetPeerTags(
			testAttrs(map[string]string{keyServerAddress: longValue}),
			ptrace.SpanKindClient,
		)
		longWant := "peer.hostname:" + longValue
		dst := make([]byte, 0, len(longWant))
		allocs := testing.AllocsPerRun(1000, func() {
			dst = longTags.AppendFlattened(dst[:0])
			peerTagsAppendSink = dst
		})
		if allocs != 0 {
			t.Fatalf("long exact-capacity AppendFlattened allocated %v times, want 0", allocs)
		}
		if string(dst) != longWant {
			t.Fatalf("long exact-capacity output length = %d, want %d", len(dst), len(longWant))
		}
	})
}

func TestPeerTagsAppendFlattenedMatchesCompatibilityAPI(t *testing.T) {
	tests := []struct {
		name  string
		attrs pcommon.Map
		kind  ptrace.SpanKind
	}{
		{name: "current", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindClient},
		{name: "legacy fallback", attrs: legacyFallbackPeerAttrs(), kind: ptrace.SpanKindProducer},
		{name: "kind gate", attrs: populatedPeerAttrs(), kind: ptrace.SpanKindServer},
		{name: "empty", attrs: pcommon.NewMap(), kind: ptrace.SpanKindConsumer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags := GetPeerTags(test.attrs, test.kind)
			got := string(tags.AppendFlattened(make([]byte, 0, 512)))
			want := GetFlattenedPeerTags(test.attrs, test.kind)
			if got != want {
				t.Fatalf("AppendFlattened() = %q, want compatibility output %q", got, want)
			}
		})
	}
}

func TestPeerTagsAppendFlattenedPreservesDestinationPrefix(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyDBSystem:      "redis",
		keyServerAddress: "localhost",
	})
	dst := make([]byte, len("prefix/"), 128)
	copy(dst, "prefix/")
	got := string(GetPeerTags(attrs, ptrace.SpanKindClient).AppendFlattened(dst))
	want := "prefix/peer.db.system:redis,peer.hostname:localhost"
	if got != want {
		t.Fatalf("AppendFlattened() = %q, want %q", got, want)
	}

	empty := dst[:len("prefix/")]
	gotEmpty := GetPeerTags(attrs, ptrace.SpanKindServer).AppendFlattened(empty)
	if string(gotEmpty) != "prefix/" || len(gotEmpty) != len(empty) {
		t.Fatalf("empty AppendFlattened() changed dst: got %q", gotEmpty)
	}
}

func TestPeerFlattenedOrderMatchesRegistry(t *testing.T) {
	concepts := make([]string, 0, peerFieldCount+1)
	seen := [peerFieldCount + 1]bool{}
	for _, offset := range peerFlattenedOrder {
		if int(offset) > peerFieldCount {
			t.Fatalf("flattened offset %d is outside peer field count %d", offset, peerFieldCount)
		}
		if seen[offset] {
			t.Fatalf("flattened offset %d appears more than once", offset)
		}
		seen[offset] = true
		if int(offset) == peerFieldCount {
			concepts = append(concepts, string(ConceptPeerPort))
			continue
		}
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		concepts = append(concepts, semconvmeta.Field(fieldID).Concept())
	}

	want := append([]string(nil), concepts...)
	sort.Strings(want)
	if !reflect.DeepEqual(concepts, want) {
		t.Fatalf("peerFlattenedOrder concepts = %v, want lexical order %v", concepts, want)
	}
}

func TestPeerTagsDoesNotExposeMutableFields(t *testing.T) {
	typeOfTags := reflect.TypeOf(PeerTags{})
	for index := 0; index < typeOfTags.NumField(); index++ {
		field := typeOfTags.Field(index)
		if field.IsExported() {
			t.Fatalf("PeerTags field %q is exported", field.Name)
		}
	}
}

// peerSourceAttrs builds a map carrying every peer.* source, including the
// string-slice DynamoDB source that testAttrs cannot express.
func peerSourceAttrs() pcommon.Map {
	attrs := testAttrs(map[string]string{
		keyPeerService:                    "checkout-service",
		keyServerAddress:                  "db.internal",
		keyDBNamespace:                    "orders",
		keyMessagingDestinationName:       "orders.created",
		keyRPCService:                     "orders.v1.OrderService",
		keyAWSS3Bucket:                    "invoice-archive",
		keyAWSKinesisStreamName:           "order-events",
		keyDBSystemName:                   "postgresql",
		keyMessagingSystem:                "kafka",
		keyRPCSystemName:                  "grpc",
		keyMessagingKafkaBootstrapServers: "broker-a:9092",
		keyMessagingRocketMQBrokerAddress: "broker-a:10911",
	})
	attrs.PutEmptySlice(keyAWSDynamoDBTableNames).AppendEmpty().SetStr("orders-table")
	return attrs
}

func attrValue(t *testing.T, attrs pcommon.Map, key string) (string, bool) {
	t.Helper()
	value, ok := attrs.Get(key)
	if !ok {
		return "", false
	}
	return value.Str(), true
}

func TestSetPeerTagsWritesPeerConcepts(t *testing.T) {
	attrs := peerSourceAttrs()
	SetPeerTags(attrs, ptrace.SpanKindClient)

	want := map[string]string{
		"peer.service":                 "checkout-service",
		"peer.hostname":                "db.internal",
		"peer.db.name":                 "orders",
		"peer.messaging.destination":   "orders.created",
		"peer.rpc.service":             "orders.v1.OrderService",
		"peer.aws.dynamodb.table":      "orders-table",
		"peer.aws.s3.bucket":           "invoice-archive",
		"peer.aws.kinesis.stream":      "order-events",
		"peer.db.system":               "postgresql",
		"peer.messaging.system":        "kafka",
		"peer.rpc.system":              "grpc",
		"peer.kafka.bootstrap.servers": "broker-a:9092",
		"peer.rocketmq.broker":         "broker-a:10911",
	}
	for key, expected := range want {
		got, ok := attrValue(t, attrs, key)
		if !ok {
			t.Errorf("%s not written", key)
			continue
		}
		if got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
}

func TestSetPeerTagsKindGate(t *testing.T) {
	// peer.service is omitted: it is both a source and a destination key, so it
	// legitimately pre-exists in the source attributes. The reflect.DeepEqual
	// invariant below is the real assertion that an excluded kind mutates nothing
	// (including not overwriting the pre-existing peer.service).
	peerKeys := []string{
		"peer.hostname", "peer.db.name", "peer.messaging.destination",
		"peer.rpc.service", "peer.aws.dynamodb.table", "peer.aws.s3.bucket", "peer.aws.kinesis.stream",
	}

	for _, kind := range []ptrace.SpanKind{
		ptrace.SpanKindServer,
		ptrace.SpanKindInternal,
		ptrace.SpanKindUnspecified,
	} {
		t.Run(kind.String(), func(t *testing.T) {
			attrs := peerSourceAttrs()
			before := attrs.AsRaw()

			SetPeerTags(attrs, kind)

			for _, key := range peerKeys {
				if _, ok := attrs.Get(key); ok {
					t.Errorf("%s written for excluded kind %s", key, kind)
				}
			}
			if after := attrs.AsRaw(); !reflect.DeepEqual(after, before) {
				t.Errorf("excluded kind mutated attributes: before=%v, after=%v", before, after)
			}
		})
	}
}

func TestSetPeerTagsSkipsEmpty(t *testing.T) {
	attrs := testAttrs(map[string]string{keyServerAddress: "db.internal"})
	SetPeerTags(attrs, ptrace.SpanKindProducer)

	if got, ok := attrValue(t, attrs, "peer.hostname"); !ok || got != "db.internal" {
		t.Fatalf("peer.hostname = %q ok=%v, want %q", got, ok, "db.internal")
	}
	for _, key := range []string{
		"peer.service", "peer.db.name", "peer.messaging.destination",
		"peer.rpc.service", "peer.aws.dynamodb.table", "peer.aws.s3.bucket", "peer.aws.kinesis.stream",
	} {
		if _, ok := attrs.Get(key); ok {
			t.Errorf("%s written from an absent source", key)
		}
	}
}

func TestSetPeerTagsIsIdempotent(t *testing.T) {
	attrs := peerSourceAttrs()

	SetPeerTags(attrs, ptrace.SpanKindConsumer)
	first := attrs.AsRaw()

	SetPeerTags(attrs, ptrace.SpanKindConsumer)
	second := attrs.AsRaw()

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("second call changed attributes: first=%v, second=%v", first, second)
	}
}

func TestSetPeerTagsPreservesOtherAttributes(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyServerAddress: "db.internal",
		keyHTTPRoute:     "/orders/{id}",
		"custom.tag":     "keep-me",
	})

	SetPeerTags(attrs, ptrace.SpanKindClient)

	if got, ok := attrValue(t, attrs, keyHTTPRoute); !ok || got != "/orders/{id}" {
		t.Errorf("%s = %q ok=%v, want preserved", keyHTTPRoute, got, ok)
	}
	if got, ok := attrValue(t, attrs, "custom.tag"); !ok || got != "keep-me" {
		t.Errorf("custom.tag = %q ok=%v, want preserved", got, ok)
	}
}

func TestGetFlattenedPeerTags(t *testing.T) {
	t.Run("sorted name:value pairs include the peer system", func(t *testing.T) {
		attrs := testAttrs(map[string]string{
			keyDBSystem:      "redis",
			keyServerAddress: "localhost",
		})
		want := "peer.db.system:redis,peer.hostname:localhost"
		if got := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient); got != want {
			t.Fatalf("flattened = %q, want %q", got, want)
		}
	})

	t.Run("kind gate excludes server", func(t *testing.T) {
		attrs := testAttrs(map[string]string{keyDBSystem: "redis", keyServerAddress: "localhost"})
		if got := GetFlattenedPeerTags(attrs, ptrace.SpanKindServer); got != "" {
			t.Fatalf("flattened = %q, want empty", got)
		}
	})

	t.Run("no peer tags yields empty", func(t *testing.T) {
		attrs := testAttrs(map[string]string{keyHTTPRoute: "/orders"})
		if got := GetFlattenedPeerTags(attrs, ptrace.SpanKindClient); got != "" {
			t.Fatalf("flattened = %q, want empty", got)
		}
	})

	t.Run("kafka bootstrap servers join the fingerprint", func(t *testing.T) {
		attrs := testAttrs(map[string]string{
			keyMessagingSystem:                "kafka",
			keyMessagingDestinationName:       "orders",
			keyMessagingKafkaBootstrapServers: "b1:9092",
		})
		want := "peer.kafka.bootstrap.servers:b1:9092,peer.messaging.destination:orders,peer.messaging.system:kafka"
		if got := GetFlattenedPeerTags(attrs, ptrace.SpanKindProducer); got != want {
			t.Fatalf("flattened = %q, want %q", got, want)
		}
	})
}

// TestFlattenedPeerTagsStableAcrossSystemRename proves the canonicalized peer
// system keeps one entity's flattened value identical whether the emitter used
// the legacy db.system value or the current db.system.name value.
func TestFlattenedPeerTagsStableAcrossSystemRename(t *testing.T) {
	legacy := testAttrs(map[string]string{keyDBSystem: "mssql", keyServerAddress: "sql-1"})
	current := testAttrs(map[string]string{keyDBSystemName: "microsoft.sql_server", keyServerAddress: "sql-1"})

	legacyFlat := GetFlattenedPeerTags(legacy, ptrace.SpanKindClient)
	currentFlat := GetFlattenedPeerTags(current, ptrace.SpanKindClient)

	if legacyFlat != currentFlat {
		t.Fatalf("flattened split across rename: legacy=%q current=%q", legacyFlat, currentFlat)
	}
	if want := "peer.db.system:microsoft.sql_server,peer.hostname:sql-1"; currentFlat != want {
		t.Fatalf("flattened = %q, want %q", currentFlat, want)
	}
}

func TestSetPeerTagsWritesFlattened(t *testing.T) {
	if keyDFFlattenedPeerTags != "_df.flattened.peer.tags" {
		t.Fatalf("keyDFFlattenedPeerTags = %q, want Project namespace", keyDFFlattenedPeerTags)
	}

	attrs := testAttrs(map[string]string{
		keyDBSystem:      "redis",
		keyServerAddress: "localhost",
	})
	SetPeerTags(attrs, ptrace.SpanKindClient)

	got, ok := attrValue(t, attrs, keyDFFlattenedPeerTags)
	if !ok {
		t.Fatalf("%s not written", keyDFFlattenedPeerTags)
	}
	if want := "peer.db.system:redis,peer.hostname:localhost"; got != want {
		t.Fatalf("%s = %q, want %q", keyDFFlattenedPeerTags, got, want)
	}
	// The peer system is also materialized as its own concept column.
	if v, _ := attrValue(t, attrs, "peer.db.system"); v != "redis" {
		t.Errorf("peer.db.system = %q, want redis", v)
	}
}

func TestSetPeerTagsRemovesStaleFlattenedWhenProjectionIsEmpty(t *testing.T) {
	attrs := testAttrs(map[string]string{
		keyDFFlattenedPeerTags: "peer.hostname:stale.example",
		keyHTTPRoute:           "/orders/{id}",
	})

	SetPeerTags(attrs, ptrace.SpanKindClient)

	if _, ok := attrs.Get(keyDFFlattenedPeerTags); ok {
		t.Fatalf("%s retained after the peer projection became empty", keyDFFlattenedPeerTags)
	}
	if got, ok := attrValue(t, attrs, keyHTTPRoute); !ok || got != "/orders/{id}" {
		t.Fatalf("%s = %q ok=%v, want unrelated attribute preserved", keyHTTPRoute, got, ok)
	}
}

func TestSetPeerTagsUsesCanonicalFlattenedKey(t *testing.T) {
	const previousKey = "otelmap.flattened.peer.tags"
	attrs := testAttrs(map[string]string{keyDBSystem: "redis", keyServerAddress: "cache.internal"})
	SetPeerTags(attrs, ptrace.SpanKindClient)
	if _, ok := attrs.Get(previousKey); ok {
		t.Fatal("SetPeerTags generated the previous flattened key")
	}
	current, ok := attrs.Get("_df.flattened.peer.tags")
	if !ok || current.Str() != "peer.db.system:redis,peer.hostname:cache.internal" {
		t.Fatalf("canonical flattened field = %v", current)
	}

	// Existing source extensions are preserved; this API does not migrate them.
	attrs.PutStr(previousKey, "legacy:unchanged")
	SetPeerTags(attrs, ptrace.SpanKindClient)
	previous, _ := attrs.Get(previousKey)
	if previous.Str() != "legacy:unchanged" {
		t.Fatal("SetPeerTags rewrote a pre-existing extension")
	}
}

func TestSetPeerTagsDynamoDBFirstNonEmpty(t *testing.T) {
	attrs := pcommon.NewMap()
	tables := attrs.PutEmptySlice(keyAWSDynamoDBTableNames)
	tables.AppendEmpty().SetStr("")
	tables.AppendEmpty().SetStr("orders-table")

	SetPeerTags(attrs, ptrace.SpanKindClient)

	if got, ok := attrValue(t, attrs, "peer.aws.dynamodb.table"); !ok || got != "orders-table" {
		t.Fatalf("peer.aws.dynamodb.table = %q ok=%v, want %q", got, ok, "orders-table")
	}
}
