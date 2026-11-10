// SPDX-License-Identifier: Apache-2.0

package otlpmap

// Concept identifies a normalized semantic concept independently of the
// attribute keys that may supply its value.
//
// A concept can have multiple ordered OTel, legacy, or vendor source keys. The
// peer concepts below follow Datadog's pkg/trace/semantics names so callers can
// distinguish the projected output from its input attributes.
type Concept string

// Datadog service and resource identification concepts used as explicit
// overrides by GetOperationName and GetResourceName.
const (
	ConceptOperationName Concept = "operation.name"
	ConceptResourceName  Concept = "resource.name"
)

// Datadog peer concepts used by SetPeerTags and GetFlattenedPeerTags.
const (
	ConceptPeerService              Concept = "peer.service"
	ConceptPeerHostname             Concept = "peer.hostname"
	ConceptPeerDBName               Concept = "peer.db.name"
	ConceptPeerDBSystem             Concept = "peer.db.system"
	ConceptPeerMessagingDestination Concept = "peer.messaging.destination"
	ConceptPeerMessagingSystem      Concept = "peer.messaging.system"
	ConceptPeerKafkaBootstrapSrvs   Concept = "peer.kafka.bootstrap.servers"
	ConceptPeerRPCService           Concept = "peer.rpc.service"
	ConceptPeerRPCSystem            Concept = "peer.rpc.system"
	ConceptPeerAWSS3Bucket          Concept = "peer.aws.s3.bucket"
	ConceptPeerAWSDynamoDBTable     Concept = "peer.aws.dynamodb.table"
	ConceptPeerAWSKinesisStream     Concept = "peer.aws.kinesis.stream"

	// ConceptPeerRocketMQBroker is a Project extension. Datadog does not
	// currently define an equivalent canonical peer concept.
	ConceptPeerRocketMQBroker Concept = "peer.rocketmq.broker"

	// ConceptPeerPort is a Project extension. It is the decimal port paired
	// with a network-fallback peer.hostname, never a configured target port.
	ConceptPeerPort Concept = "peer.port"
)
