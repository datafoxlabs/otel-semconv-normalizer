// SPDX-License-Identifier: Apache-2.0

package otlpmap

import "testing"

func TestConceptValues(t *testing.T) {
	tests := []struct {
		name string
		got  Concept
		want string
	}{
		{name: "operation name", got: ConceptOperationName, want: "operation.name"},
		{name: "resource name", got: ConceptResourceName, want: "resource.name"},
		{name: "peer service", got: ConceptPeerService, want: "peer.service"},
		{name: "peer hostname", got: ConceptPeerHostname, want: "peer.hostname"},
		{name: "peer port", got: ConceptPeerPort, want: "peer.port"},
		{name: "peer DB name", got: ConceptPeerDBName, want: "peer.db.name"},
		{name: "peer DB system", got: ConceptPeerDBSystem, want: "peer.db.system"},
		{name: "peer messaging destination", got: ConceptPeerMessagingDestination, want: "peer.messaging.destination"},
		{name: "peer messaging system", got: ConceptPeerMessagingSystem, want: "peer.messaging.system"},
		{name: "peer Kafka bootstrap servers", got: ConceptPeerKafkaBootstrapSrvs, want: "peer.kafka.bootstrap.servers"},
		{name: "peer RPC service", got: ConceptPeerRPCService, want: "peer.rpc.service"},
		{name: "peer RPC system", got: ConceptPeerRPCSystem, want: "peer.rpc.system"},
		{name: "peer AWS S3 bucket", got: ConceptPeerAWSS3Bucket, want: "peer.aws.s3.bucket"},
		{name: "peer AWS DynamoDB table", got: ConceptPeerAWSDynamoDBTable, want: "peer.aws.dynamodb.table"},
		{name: "peer AWS Kinesis stream", got: ConceptPeerAWSKinesisStream, want: "peer.aws.kinesis.stream"},
		{name: "peer RocketMQ broker extension", got: ConceptPeerRocketMQBroker, want: "peer.rocketmq.broker"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.got) != test.want {
				t.Fatalf("concept = %q, want %q", test.got, test.want)
			}
		})
	}
}
