// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func isPeerSpanKind(spanKind ptrace.SpanKind) bool {
	switch spanKind {
	case ptrace.SpanKindClient, ptrace.SpanKindProducer, ptrace.SpanKindConsumer:
		return true
	default:
		return false
	}
}

func getPeerField(signalAttrs pcommon.Map, spanKind ptrace.SpanKind, id semconvmeta.FieldID) string {
	field := semconvmeta.Field(id)
	if field.KindPolicy() == semconvmeta.KindPeer && !isPeerSpanKind(spanKind) {
		return ""
	}

	switch field.ValueType() {
	case semconvmeta.ValueTypeString:
		value := getCanonicalStringField(signalAttrs, id)
		if field.ValuePolicy() == semconvmeta.ValueTechnology {
			return semconvmeta.CanonicalizeTechnology(value)
		}
		return value
	case semconvmeta.ValueTypeStringSlice:
		for i, count := uint8(0), field.SourceKeyCount(); i < count; i++ {
			value, ok := signalAttrs.Get(field.SourceKey(i))
			if !ok || value.Type() != pcommon.ValueTypeSlice {
				continue
			}
			values := value.Slice()
			for j := 0; j < values.Len(); j++ {
				element := values.At(j)
				if element.Type() == pcommon.ValueTypeStr && element.Str() != "" {
					return element.Str()
				}
			}
		}
	}
	return ""
}

// GetPeerService returns the explicit peer.service value for an outbound span.
// The source attribute was removed from SemConv v1.39 but remains supported as
// a widely emitted legacy signal.
func GetPeerService(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerService)
}

// GetPeerHostname returns server.address, falling back to net.peer.name.
// If no logical peer target is available, it returns the network address
// selected by GetPeerTags. Composite targets are preserved verbatim; ports
// are never appended to this value.
func GetPeerHostname(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	if !isPeerSpanKind(spanKind) {
		return ""
	}
	if target := getCanonicalStringField(signalAttrs, semconvmeta.FieldPeerHostname); target != "" {
		return target
	}
	return getPeerTags(signalAttrs).attributes.values[semconvmeta.FieldPeerHostname-semconvmeta.CoreFieldCount]
}

// GetPeerPort returns the decimal network port paired with a fallback hostname.
// It returns empty for logical targets, missing addresses or invalid ports.
// Callers that also need other peer fields should reuse GetPeerTags instead.
func GetPeerPort(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	if !isPeerSpanKind(spanKind) {
		return ""
	}
	return getPeerTags(signalAttrs).port
}

// GetPeerDBName returns db.namespace, falling back to legacy db.name.
func GetPeerDBName(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerDBName)
}

// GetPeerMessagingDestination returns messaging.destination.name, falling back
// to the legacy messaging.destination attribute.
func GetPeerMessagingDestination(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerMessagingDestination)
}

// GetPeerRPCService returns rpc.service. The source attribute was removed from
// SemConv v1.39 but remains supported within the project's compatibility window.
func GetPeerRPCService(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerRPCService)
}

// GetPeerAWSDynamoDBTable returns the first non-empty string from
// aws.dynamodb.table_names. SemConv defines the source as a string array, while
// this package emits one value per field. Selecting the first non-empty element
// is deterministic and preserves the hot path's zero-allocation contract;
// callers that need the complete batch must retain the original attribute.
func GetPeerAWSDynamoDBTable(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerAWSDynamoDBTable)
}

// GetPeerAWSS3Bucket returns the raw aws.s3.bucket value.
func GetPeerAWSS3Bucket(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerAWSS3Bucket)
}

// GetPeerAWSKinesisStream returns the raw aws.kinesis.stream_name value.
func GetPeerAWSKinesisStream(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerAWSKinesisStream)
}

// GetPeerDBSystem returns the peer's database system (db.system.name, falling
// back to db.system / db.type), spelling-normalized so the peer fingerprint is
// stable across SemConv value renames. It discriminates the peer entity.
func GetPeerDBSystem(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerDBSystem)
}

// GetPeerMessagingSystem returns the peer's messaging system (messaging.system).
func GetPeerMessagingSystem(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerMessagingSystem)
}

// GetPeerRPCSystem returns the peer's RPC system (rpc.system.name, falling back
// to rpc.system), spelling-normalized.
func GetPeerRPCSystem(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerRPCSystem)
}

// GetPeerKafkaBootstrapServers returns the Kafka broker set from the experimental
// messaging.kafka.bootstrap.servers attribute (emitted by OTel Java's
// KafkaProducer, read by DataDog as a peer tag). Empty when the emitter did not
// enable that experimental attribute.
func GetPeerKafkaBootstrapServers(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerKafkaBootstrapServers)
}

// GetPeerRocketMQBroker returns the RocketMQ broker from the experimental
// messaging.rocketmq.broker_address attribute (emitted by OTel Java's RocketMQ
// producer and consumer). Empty when the emitter did not enable that
// experimental attribute.
func GetPeerRocketMQBroker(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	return getPeerField(signalAttrs, spanKind, semconvmeta.FieldPeerRocketMQBroker)
}
