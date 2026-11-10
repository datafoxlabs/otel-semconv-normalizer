// SPDX-License-Identifier: Apache-2.0

// Package otlpmap maps OpenTelemetry (OTLP) span and resource attributes to the
// flat fields you persist per span — e.g. for storage into ClickHouse. The two
// classification fields are orthogonal columns, one per axis:
//
//	GetTechnology  the "which system" axis: the raw backing technology
//	               (mysql, clickhouse, redis, kafka, grpc, ...), spelling-
//	               normalized across semconv generations; "http" for a bare
//	               HTTP span; "unknown" when there is no backing system. Lossless —
//	               no DD-style collapse of SQL engines to "sql".
//	EntityType     the "which role" axis: the coarse entity type
//	               (web/http/db/cache/queue/rpc/serverless/custom), for GROUP BY
//	               and icons. GetEntityType always resolves, even with no
//	               backing system.
//	GetOperationName / GetResourceName / GetServiceName / GetDeploymentEnvironmentName / GetServiceVersion
//	GetHTTPRequestMethod / GetHTTPRequestMethodOriginal
//	GetHTTPResponseStatusCode
//	GetURLFull / GetURLScheme / GetURLTemplate / GetHTTPRoute
//	GetUserAgentOriginal / GetNetworkProtocolName / GetNetworkTransport
//	GetDBOperationName / GetDBCollectionName / GetDBNamespace
//	GetDBQuerySummary / GetDBQueryText
//	GetPeerService / GetPeerHostname / GetPeerPort / GetPeerDBName
//	GetPeerMessagingDestination / GetPeerRPCService
//	GetPeerAWSDynamoDBTable / GetPeerAWSS3Bucket / GetPeerAWSKinesisStream
//	GetPeerDBSystem / GetPeerMessagingSystem / GetPeerRPCSystem
//	GetPeerKafkaBootstrapServers / GetPeerRocketMQBroker
//	GetPeerTags / RangePeerAttributes / GetFlattenedPeerTags / SetPeerTags / GetPeerName
//
// It mirrors the shape of DataDog/datadog-agent's pkg/opentelemetry-mapping-go:
// a handful of extraction functions over an attribute map, no framework. The
// one substantive difference is that every lookup lists the current semconv
// attribute name first and older names as fallbacks (via GetOTelAttrVal), so a
// span emitted with OTEL_SEMCONV_STABILITY_OPT_IN set still resolves.
//
// Public APIs accept collector pdata's pcommon.Map and ptrace.SpanKind directly,
// so Collector call sites can pass span.Attributes() and span.Kind() without an
// adapter, duplicate enum, or intermediate attribute map.
//
// Normalized output identifiers use the distinct Concept type. Datadog-defined
// operation, resource, and peer concepts retain their Concept* names, while
// their ordered OTel, legacy, or vendor input attributes remain plain string
// keys. This prevents a projected concept such as ConceptPeerHostname from
// being confused with its server.address or net.peer.name sources.
//
// FaaS attributes deliberately classify only the role in the current API.
// GetEntityType recognizes faas.trigger or faas.invoked_name as serverless, but
// GetTechnology does not derive a technology from faas.invoked_provider (cloud
// vendor), faas.invoked_name (function identity), or faas.trigger (trigger
// category). With no independently recognized DB, messaging, RPC, or HTTP
// marker, its result is therefore "unknown". The Resource attribute cloud.platform
// describes the telemetry producer's local execution platform, which
// GetTechnology's signal-only input cannot see. It may describe an incoming
// function execution environment, but on an outgoing invocation it describes
// the caller and must not be used as the remote target platform.
//
// Mixed signals follow the existing classifier order: DB, messaging, and RPC
// precede FaaS in both columns; FaaS precedes HTTP only in GetEntityType.
// Consequently FaaS+HTTP is "http" + serverless, while FaaS+DB is the DB
// technology + db/cache. A provider without a non-empty trigger or invoked name
// is not by itself a FaaS role marker.
//
// HTTP request methods follow the OTel canonical/original pair: unknown methods
// become _OTHER while GetHTTPRequestMethodOriginal preserves the wire value.
// Legacy network transport values ip_tcp and ip_udp become tcp and udp. The
// core getters do not mutate the source map or dynamically interpret schemas.
//
// GetURLFull and GetUserAgentOriginal return raw, potentially sensitive or
// high-cardinality values. Callers must redact URL credentials and sensitive
// query content before persistence, and should prefer GetHTTPRoute or
// GetURLTemplate for low-cardinality grouping. Kind-dependent address mapping,
// http.target splitting, protocol-version reshaping, and body-size opt-in are
// intentionally outside this flat core-field API.
//
// Database core getters normalize the semconv 1.30 migration without rewriting
// attributes. GetOperationName uses db.operation.name when present, while
// GetResourceName follows Datadog resource naming and uses db.query.text (or
// legacy db.statement), then its fallback name. The canonical caller passes
// ptrace.Span.Name() as that fallback. The project distribution guarantees
// agent-side database query sanitization before export; this package does not
// parse or sanitize SQL. GetDBQueryText remains an independent field, and the
// caller-provided fallback is not sanitized by this package.
//
// Removed db.connection_string and db.user attributes are never consumed.
// Context-dependent db.instance.id remapping and SQL-derived query summaries are
// intentionally outside this flat registry.
//
// Peer getters expose independent, raw peer.* columns only for Client, Producer,
// and Consumer spans. Server, Internal, and Unspecified spans return empty even
// when they carry operation or service overrides. The getters consolidate
// server.address over legacy net.peer.name and db.namespace over legacy db.name;
// the three peer system getters (GetPeerDBSystem, GetPeerMessagingSystem,
// GetPeerRPCSystem) spelling-normalize their value like GetTechnology. Sources are
// OTel-native keys except GetPeerKafkaBootstrapServers and GetPeerRocketMQBroker,
// which read the experimental, non-stable-SemConv messaging.kafka.bootstrap.servers
// and messaging.rocketmq.broker_address that OTel Java's Kafka and RocketMQ
// instrumentation emit (Kafka migrates to messaging.kafka.cluster.id once that
// stable attribute lands in the canonical version). The DynamoDB table getter returns the
// first non-empty string from aws.dynamodb.table_names to preserve a deterministic
// single value without allocation. These read-only getters never mutate the map.
// GetPeerEndpoints exposes logical Target and contacted Network endpoints with
// separate, validated optional ports; compound target strings stay opaque.
//
// GetPeerTags preserves the existing 13 concepts and adds Project's peer.port.
// With a logical target it scans the source map once without allocation. With
// no logical target it fills peer.hostname from the Network endpoint and writes
// its valid paired port as a decimal string; formatting the port may allocate.
// Database names, topics and RPC names alone do not block this fallback.
// A consumer that needs both
// independent concepts and the flattened representation should call Range and
// AppendFlattened on that snapshot: with a sufficiently preallocated byte
// destination, those two methods perform no allocation. The snapshot
// retains selected strings but does not retain or mutate the source map.
// RangePeerAttributes remains the compatible one-operation wrapper.
//
// SetPeerTags is an explicit, opt-in write-back: it PutStr's each present
// peer concept and writes the sorted, comma-joined flattened set
// under the Project-owned _df.flattened.peer.tags key (see
// GetFlattenedPeerTags). It mutates the map, so it allocates and lives outside
// the read core; it does not group spans into inferred entities.
//
// GetPeerName chooses one downstream display name by DataDog's naming precedence
// (peer.service first; then per domain: database db.name over DynamoDB table over
// hostname over system, queue destination over Kafka/RocketMQ/Kinesis identifiers
// over system, service rpc.service over S3 bucket over hostname). It is read-only
// and deliberately NOT wired into SetPeerTags: naming precedence is a presentation
// policy best computed at query time (a ClickHouse view over the stored peer.*
// columns), and GetPeerName is its authoritative, contract-tested definition that
// such a view mirrors.
//
// Peer extraction does not aggregate statistics, construct a service graph,
// append server.port, or quantize IP addresses. DataDog-private and out-of-window
// sources are deliberately ignored. Address redaction and cardinality control
// belong in a later additive persistence layer; peer system/type also reuse
// GetTechnology and GetEntityType.
package otlpmap
