// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	semconv1_25 "go.opentelemetry.io/otel/semconv/v1.25.0"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_34 "go.opentelemetry.io/otel/semconv/v1.34.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Attribute keys, sourced from official go.opentelemetry.io/otel/semconv
// constants wherever they exist. Current stable names come from the project's
// canonical v1.41.0. Removed legacy names come from the newest retained package
// that still exposes them, or remain audited string literals when no current Go
// constant exists. Keeping them as ordered fallbacks lets pre-migration spans
// resolve without rewriting the input map.
const (
	// Technology discriminators.
	keyDBSystemName    = string(semconv1_41.DBSystemNameKey) // db.system.name (>=1.30)
	keyDBSystem        = string(semconv1_27.DBSystemKey)     // db.system (pre-1.30)
	keyDBType          = "db.type"                           // legacy vendor integration attribute
	keyMessagingSystem = string(semconv1_41.MessagingSystemKey)
	keyRPCSystem       = string(semconv1_34.RPCSystemKey)     // rpc.system
	keyRPCSystemName   = string(semconv1_41.RPCSystemNameKey) // rpc.system.name (>=1.39)

	// HTTP / URL.
	keyHTTPRequestMethod         = string(semconv1_41.HTTPRequestMethodKey)         // http.request.method (>=1.23)
	keyHTTPRequestMethodOriginal = string(semconv1_41.HTTPRequestMethodOriginalKey) // http.request.method_original
	keyHTTPMethod                = string(semconv1_25.HTTPMethodKey)                // http.method (pre-1.23)
	keyHTTPResponseStatusCode    = string(semconv1_41.HTTPResponseStatusCodeKey)    // http.response.status_code (>=1.23)
	keyHTTPStatusCode            = string(semconv1_25.HTTPStatusCodeKey)            // http.status_code (pre-1.23)
	keyHTTPRoute                 = string(semconv1_41.HTTPRouteKey)
	keyURLFull                   = string(semconv1_41.URLFullKey) // url.full (>=1.23)
	keyHTTPURL                   = string(semconv1_25.HTTPURLKey) // http.url (pre-1.23)
	keyURLScheme                 = string(semconv1_41.URLSchemeKey)
	keyHTTPScheme                = string(semconv1_25.HTTPSchemeKey) // http.scheme (pre-1.23)
	keyURLTemplate               = string(semconv1_41.URLTemplateKey)
	keyUserAgentOriginal         = string(semconv1_41.UserAgentOriginalKey)
	keyHTTPUserAgent             = string(semconv1_25.HTTPUserAgentKey)

	// RPC.
	keyRPCMethod  = string(semconv1_41.RPCMethodKey)
	keyRPCService = string(semconv1_34.RPCServiceKey)

	// Messaging.
	keyMessagingOperationType   = string(semconv1_41.MessagingOperationTypeKey)
	keyMessagingOperationName   = string(semconv1_41.MessagingOperationNameKey)
	keyMessagingOperation       = "messaging.operation" // legacy
	keyMessagingDestinationName = string(semconv1_41.MessagingDestinationNameKey)
	keyMessagingDestination     = "messaging.destination" // legacy

	// FaaS.
	keyFaaSTrigger         = string(semconv1_41.FaaSTriggerKey)
	keyFaaSInvokedName     = string(semconv1_41.FaaSInvokedNameKey)
	keyFaaSInvokedProvider = string(semconv1_41.FaaSInvokedProviderKey)

	// GraphQL.
	keyGraphQLOperationType = string(semconv1_41.GraphQLOperationTypeKey)
	keyGraphQLOperationName = string(semconv1_41.GraphQLOperationNameKey)

	// Database core fields.
	keyDBOperationName      = string(semconv1_41.DBOperationNameKey)
	keyDBOperation          = string(semconv1_25.DBOperationKey)
	keyDBCollectionName     = string(semconv1_41.DBCollectionNameKey)
	keyDBSQLTable           = string(semconv1_25.DBSQLTableKey)
	keyDBCassandraTable     = string(semconv1_25.DBCassandraTableKey)
	keyDBMongoDBCollection  = string(semconv1_25.DBMongoDBCollectionKey)
	keyDBCosmosDBContainer  = string(semconv1_25.DBCosmosDBContainerKey)
	keyDBNamespace          = string(semconv1_41.DBNamespaceKey)
	keyDBName               = string(semconv1_25.DBNameKey)
	keyDBRedisDatabaseIndex = string(semconv1_25.DBRedisDBIndexKey)
	keyDBMSSQLInstanceName  = string(semconv1_25.DBMSSQLInstanceNameKey)
	keyDBQuerySummary       = string(semconv1_41.DBQuerySummaryKey)
	keyDBQueryText          = string(semconv1_41.DBQueryTextKey)
	keyDBStatement          = string(semconv1_25.DBStatementKey)

	// Peer projection sources. peer.service and net.peer.name were removed
	// from newer SemConv packages, so they remain anchored to v1.25.
	keyPeerService           = string(semconv1_25.PeerServiceKey)
	keyServerAddress         = string(semconv1_41.ServerAddressKey)
	keyNetPeerName           = string(semconv1_25.NetPeerNameKey)
	keyAWSDynamoDBTableNames = string(semconv1_41.AWSDynamoDBTableNamesKey)
	keyAWSS3Bucket           = string(semconv1_41.AWSS3BucketKey)
	keyAWSKinesisStreamName  = string(semconv1_41.AWSKinesisStreamNameKey)

	// keyMessagingKafkaBootstrapServers is an experimental attribute (not stable
	// SemConv, so no Go constant): mainline OTel Java's KafkaProducer emits it via
	// its ExperimentalAttributesExtractor, and DataDog reads it as a peer tag. It
	// identifies the Kafka broker set. Migrate to the stable messaging.kafka.cluster.id
	// once that attribute lands in the project's canonical SemConv version.
	keyMessagingKafkaBootstrapServers = "messaging.kafka.bootstrap.servers"
	// keyMessagingRocketMQBrokerAddress is likewise experimental (not stable
	// SemConv): OTel Java's RocketMQ producer and consumer emit it via their
	// ExperimentalAttributeExtractor as the broker address handling the message.
	keyMessagingRocketMQBrokerAddress = "messaging.rocketmq.broker_address"
	// keyDFFlattenedPeerTags is a Project extension, not an OTel or Datadog
	// attribute. It stores the sorted, comma-joined Datadog peer concepts emitted
	// by SetPeerTags; grouping remains a downstream concern.
	keyDFFlattenedPeerTags = "_df.flattened.peer.tags"
	// Peer system sources reuse the db/messaging/rpc system keys declared above.

	keyNetworkProtocolName = string(semconv1_41.NetworkProtocolNameKey)
	keyNetProtocolName     = string(semconv1_25.NetProtocolNameKey) // legacy (pre-1.23)
	keyNetworkTransport    = string(semconv1_41.NetworkTransportKey)
	keyNetTransport        = string(semconv1_25.NetTransportKey) // legacy (pre-1.23)

	// Resource-level.
	keyServiceName               = string(semconv1_41.ServiceNameKey)
	keyServiceVersion            = string(semconv1_41.ServiceVersionKey)
	keyDeploymentEnvironmentName = string(semconv1_41.DeploymentEnvironmentNameKey) // deployment.environment.name (>=1.27)
	keyDeploymentEnvironment     = "deployment.environment"                         // legacy

	// DataDog-specific override fields (not OTel attributes). These let an
	// emitter force the derived value; GetOperationName/GetResourceName honor them.
	// There is deliberately no span.type override — GetTechnology is a clean,
	// derived axis and must not accept an arbitrary (possibly role-valued) tag.
	keyDDOperationName = string(ConceptOperationName)
	keyDDResourceName  = string(ConceptResourceName)
)
