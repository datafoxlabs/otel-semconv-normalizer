// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// peerFlattenedOrder is the package-private static lexical order of the peer
// concept names. Keeping sorting out of the per-span hot path avoids repeated
// work. Registry tests assert that it remains complete and sorted as concepts
// evolve.
var peerFlattenedOrder = [peerFieldCount + 1]uint8{
	5,                     // peer.aws.dynamodb.table
	7,                     // peer.aws.kinesis.stream
	6,                     // peer.aws.s3.bucket
	2,                     // peer.db.name
	8,                     // peer.db.system
	1,                     // peer.hostname
	11,                    // peer.kafka.bootstrap.servers
	3,                     // peer.messaging.destination
	9,                     // peer.messaging.system
	uint8(peerFieldCount), // peer.port, owned by the identity extension
	12,                    // peer.rocketmq.broker
	4,                     // peer.rpc.service
	10,                    // peer.rpc.system
	0,                     // peer.service
}

// AppendFlattened appends the snapshot as lexically sorted "name:value" pairs
// joined by commas. It preserves any existing dst prefix and performs no
// allocation when dst has sufficient spare capacity. It does not escape or
// transform delimiters, preserving the existing flattened wire format.
func (tags PeerTags) AppendFlattened(dst []byte) []byte {
	wrotePair := false
	for _, offset := range peerFlattenedOrder {
		if int(offset) == peerFieldCount {
			if tags.port != "" {
				if wrotePair {
					dst = append(dst, ',')
				}
				dst = append(dst, string(ConceptPeerPort)...)
				dst = append(dst, ':')
				dst = append(dst, tags.port...)
				wrotePair = true
			}
			continue
		}
		if tags.attributes.present&(uint32(1)<<offset) == 0 {
			continue
		}
		if wrotePair {
			dst = append(dst, ',')
		}
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		field := semconvmeta.Field(fieldID)
		value := tags.attributes.values[offset]
		dst = append(dst, field.Concept()...)
		dst = append(dst, ':')
		dst = append(dst, value...)
		wrotePair = true
	}
	return dst
}

func (tags PeerTags) flattenedSize() int {
	size := 0
	for _, offset := range peerFlattenedOrder {
		if int(offset) == peerFieldCount || tags.attributes.present&(uint32(1)<<offset) == 0 {
			continue
		}
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		field := semconvmeta.Field(fieldID)
		value := tags.attributes.values[offset]
		if size != 0 {
			size++
		}
		size += len(field.Concept()) + 1 + len(value)
	}
	if tags.port != "" {
		if size != 0 {
			size++
		}
		size += len(ConceptPeerPort) + 1 + len(tags.port)
	}
	return size
}

// GetFlattenedPeerTags returns the flattened peer tags of an outbound span: the
// matching peer.* concepts as sorted "name:value" pairs joined by commas (e.g.
// "peer.db.system:redis,peer.hostname:localhost"). The returned value is stored
// under the Project-owned _df.flattened.peer.tags key by SetPeerTags; Datadog
// computes the equivalent matching peer-tag set internally rather than emitting
// this attribute.
//
// It is read-only but returns an allocated string when tags are present.
// Computing it is a per-span, stateless operation; grouping spans by it into an
// inferred entity is a separate downstream step and is out of scope here. Only
// client/producer/consumer spans yield a value; others return "".
func GetFlattenedPeerTags(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	tags := GetPeerTags(signalAttrs, spanKind)
	size := tags.flattenedSize()
	if size == 0 {
		return ""
	}
	return string(tags.AppendFlattened(make([]byte, 0, size)))
}

// SetPeerTags writes the peer.* projection of an outbound span back onto
// signalAttrs. Alongside the existing peer concepts, missing logical targets
// may produce a network hostname and Project's peer.port extension.
//
// Unlike the read-only GetPeer* getters, SetPeerTags MUTATES signalAttrs. It is an
// explicit, opt-in step kept out of the read core: PutStr grows the map and
// allocates, so callers invoke it at an ingestion or landing
// boundary, never on the hot read path.
//
// It writes each present, non-empty peer.* concept under its concept name
// (peer.service, peer.hostname, peer.db.system, ...) and the flattened set under
// _df.flattened.peer.tags. Values are not transformed again here: no IP
// quantization, cardinality bounding, or peer.service length normalization is
// applied. The distribution contract requires the OTel Agent to sanitize IP
// data before export; deployment-owned grouping stays downstream.
//
// The write is idempotent and leaves every non-peer attribute untouched. Only
// client/producer/consumer spans are written; others are a no-op.
func SetPeerTags(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) {
	if !isPeerSpanKind(spanKind) {
		return
	}
	tags := getPeerTags(signalAttrs)
	if tags.port == "" {
		signalAttrs.Remove(string(ConceptPeerPort))
	}
	tags.Range(func(concept Concept, value string) bool {
		key := string(concept)
		signalAttrs.PutStr(key, value)
		return true
	})
	size := tags.flattenedSize()
	if size == 0 {
		signalAttrs.Remove(keyDFFlattenedPeerTags)
		return
	}
	flattened := tags.AppendFlattened(make([]byte, 0, size))
	signalAttrs.PutStr(keyDFFlattenedPeerTags, string(flattened))
}
