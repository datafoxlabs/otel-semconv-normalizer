// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"math/bits"
	"strconv"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const peerFieldCount = int(semconvmeta.FieldCount - semconvmeta.CoreFieldCount)

// A database name, topic or RPC service name alone does not identify a
// deployment. Explicit logical targets and named cloud resources do.
const logicalPeerTargetMask uint32 = 1<<(semconvmeta.FieldPeerService-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerHostname-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerKafkaBootstrapServers-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerRocketMQBroker-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerAWSDynamoDBTable-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerAWSS3Bucket-semconvmeta.CoreFieldCount) |
	1<<(semconvmeta.FieldPeerAWSKinesisStream-semconvmeta.CoreFieldCount)

type peerAttributeValues struct {
	values     [peerFieldCount]string
	priorities [peerFieldCount]uint8
	present    uint32
}

// PeerTags is an immutable value snapshot of the non-empty peer concepts
// derived from one span. Its fields are deliberately private: callers obtain a
// snapshot with GetPeerTags and consume it through Range or AppendFlattened.
// The snapshot retains selected string values, not the source pcommon.Map.
type PeerTags struct {
	attributes peerAttributeValues
	// Port is a conditional identity extension, not a flat SemConv key alias.
	port string
}

// GetPeerTags returns a read-only snapshot of every non-empty peer concept.
// Existing logical targets use one source scan without allocation. Only when
// none is available does it read network endpoint aliases and fill hostname
// and, if present, a decimal port string. Formatting that port may allocate.
// It does not mutate signalAttrs. Only client/producer/consumer spans yield data.
func GetPeerTags(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) PeerTags {
	if signalAttrs.Len() == 0 || !isPeerSpanKind(spanKind) {
		return PeerTags{}
	}
	return getPeerTags(signalAttrs)
}

func getPeerTags(signalAttrs pcommon.Map) PeerTags {
	var tags PeerTags
	signalAttrs.Range(func(key string, value pcommon.Value) bool {
		fieldID, priority, ok := peerFieldForSourceKey(key)
		if !ok {
			return true
		}
		tags.attributes.set(fieldID, priority, peerSourceValue(fieldID, value))
		return true
	})
	tags.attributes.canonicalizeTechnologies()
	if tags.attributes.present&logicalPeerTargetMask != 0 {
		return tags
	}

	endpoint := getNetworkPeerEndpoint(signalAttrs)
	if endpoint.Address == "" {
		return tags
	}
	tags.attributes.set(semconvmeta.FieldPeerHostname, 0, endpoint.Address)
	if endpoint.HasPort {
		tags.port = strconv.Itoa(int(endpoint.Port))
	}
	return tags
}

// Range calls yield in the existing semconvmeta field order, followed by the
// optional peer.port extension. Returning false stops iteration. No allocation
// occurs during iteration.
func (tags PeerTags) Range(yield func(Concept, string) bool) {
	// Clear the lowest set bit on each iteration, preserving field order while
	// skipping absent concepts.
	for present := tags.attributes.present; present != 0; present &= present - 1 {
		offset := bits.TrailingZeros32(present)
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		field := semconvmeta.Field(fieldID)
		value := tags.attributes.values[offset]
		if !yield(Concept(field.Concept()), value) {
			return
		}
	}
	if tags.port != "" {
		yield(ConceptPeerPort, tags.port)
	}
}

// RangePeerAttributes calls yield for every non-empty peer concept derived
// from signalAttrs using the same target-first contract as GetPeerTags. It
// does not mutate input and uses the same stable order as PeerTags.Range.
// Returning false stops iteration.
func RangePeerAttributes(
	signalAttrs pcommon.Map,
	spanKind ptrace.SpanKind,
	yield func(Concept, string) bool,
) {
	if signalAttrs.Len() == 0 || !isPeerSpanKind(spanKind) {
		return
	}
	getPeerTags(signalAttrs).Range(yield)
}

func (values *peerAttributeValues) set(
	fieldID semconvmeta.FieldID,
	priority uint8,
	value string,
) {
	if value == "" {
		return
	}
	offset := int(fieldID - semconvmeta.CoreFieldCount)
	bit := uint32(1) << offset
	storedPriority := priority + 1
	if values.present&bit != 0 && values.priorities[offset] <= storedPriority {
		return
	}
	values.values[offset] = value
	values.priorities[offset] = storedPriority
	values.present |= bit
}

func (values *peerAttributeValues) canonicalizeTechnologies() {
	for present := values.present; present != 0; present &= present - 1 {
		offset := bits.TrailingZeros32(present)
		fieldID := semconvmeta.CoreFieldCount + semconvmeta.FieldID(offset)
		if semconvmeta.Field(fieldID).ValuePolicy() == semconvmeta.ValueTechnology {
			values.values[offset] = semconvmeta.CanonicalizeTechnology(values.values[offset])
		}
	}
}

func peerSourceValue(fieldID semconvmeta.FieldID, value pcommon.Value) string {
	switch semconvmeta.Field(fieldID).ValueType() {
	case semconvmeta.ValueTypeString:
		return value.AsString()
	case semconvmeta.ValueTypeStringSlice:
		if value.Type() != pcommon.ValueTypeSlice {
			return ""
		}
		values := value.Slice()
		for index := 0; index < values.Len(); index++ {
			element := values.At(index)
			if element.Type() == pcommon.ValueTypeStr && element.Str() != "" {
				return element.Str()
			}
		}
	}
	return ""
}

func peerFieldForSourceKey(key string) (semconvmeta.FieldID, uint8, bool) {
	switch key {
	case keyPeerService:
		return semconvmeta.FieldPeerService, 0, true
	case keyServerAddress:
		return semconvmeta.FieldPeerHostname, 0, true
	case keyNetPeerName:
		return semconvmeta.FieldPeerHostname, 1, true
	case keyDBNamespace:
		return semconvmeta.FieldPeerDBName, 0, true
	case keyDBName:
		return semconvmeta.FieldPeerDBName, 1, true
	case keyMessagingDestinationName:
		return semconvmeta.FieldPeerMessagingDestination, 0, true
	case keyMessagingDestination:
		return semconvmeta.FieldPeerMessagingDestination, 1, true
	case keyRPCService:
		return semconvmeta.FieldPeerRPCService, 0, true
	case keyAWSDynamoDBTableNames:
		return semconvmeta.FieldPeerAWSDynamoDBTable, 0, true
	case keyAWSS3Bucket:
		return semconvmeta.FieldPeerAWSS3Bucket, 0, true
	case keyAWSKinesisStreamName:
		return semconvmeta.FieldPeerAWSKinesisStream, 0, true
	case keyDBSystemName:
		return semconvmeta.FieldPeerDBSystem, 0, true
	case keyDBSystem:
		return semconvmeta.FieldPeerDBSystem, 1, true
	case keyDBType:
		return semconvmeta.FieldPeerDBSystem, 2, true
	case keyMessagingSystem:
		return semconvmeta.FieldPeerMessagingSystem, 0, true
	case keyRPCSystemName:
		return semconvmeta.FieldPeerRPCSystem, 0, true
	case keyRPCSystem:
		return semconvmeta.FieldPeerRPCSystem, 1, true
	case keyMessagingKafkaBootstrapServers:
		return semconvmeta.FieldPeerKafkaBootstrapServers, 0, true
	case keyMessagingRocketMQBrokerAddress:
		return semconvmeta.FieldPeerRocketMQBroker, 0, true
	default:
		return 0, 0, false
	}
}
