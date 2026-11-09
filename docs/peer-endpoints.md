# Peer Endpoint extraction (Issue #1)

## Why this API exists

Outbound spans sometimes contain an actual network peer IP but no `server.address`.
The legacy `GetPeerHostname` and the 13 existing `GetPeerTags` concepts only
project `server.address` or `net.peer.name` as `peer.hostname`. They do not
project `network.peer.address` or either endpoint's port.

`GetPeerEndpoints` is a **read-only additive API** for retrieving those facts.
It does not change `peer.hostname`, `GetPeerTags`, `SetPeerTags`,
`GetFlattenedPeerTags`, or the flattened fingerprint. Callers must explicitly
consume the new API; a deployment that persists only existing `peer.*` fields
will not start storing endpoint data automatically.

## Semantic model

| Returned field | Ordered input source keys | Meaning |
| --- | --- | --- |
| `Target.Address` | `server.address` → `net.peer.name` | Configured/logical target; may be a compound database target |
| `Target.Port` | `server.port` → `net.peer.port` | Configured server port, if present and valid |
| `Network.Address` | `network.peer.address` → `net.sock.peer.addr` → `net.peer.ip` | Address of the contacted network peer |
| `Network.Port` | `network.peer.port` → `net.sock.peer.port` | Network connection port, if present and valid |

These aliases follow the [OTel legacy network attribute registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/network/).
In particular, **`net.peer.port` on CLIENT spans maps to `server.port`**, not
`network.peer.port`. A legacy `net.peer.ip` plus `net.peer.port`
therefore does *not* create an inferred socket address-and-port pair. No source
is reassigned across the target/network boundary.

When both canonical and legacy address attributes are present but disagree,
the preferred address is retained. A missing/invalid **canonical** port is not
filled from the **legacy** port if that port would be paired with a conflicting
legacy address. In that case `HasPort=false` (and `Port=0`), rather than an
invented endpoint. The inverse is not needed: a valid canonical port remains
paired with its canonical address, even if obsolete legacy addresses disagree.
Missing alternate addresses do not by themselves prove a conflict.

Example: `network.peer.address=192.0.2.10`,
`net.sock.peer.addr=192.0.2.20`, and `net.sock.peer.port=6380` yield
`Network.Address=192.0.2.10` and `Network.HasPort=false`.
The same rule applies to conflicting `server.address` vs
`net.peer.name` when only `net.peer.port` is available.

Only valid integer ports in [1, 65535] are accepted. `HasPort` distinguishes
missing/invalid ports from a present port. Both endpoints can be partially
populated; this API does not infer missing addresses, perform DNS resolution or
insert default Redis ports. Attribute strings are preserved verbatim, including
IPv6 literals, Unix socket paths, and compound DB targets.

Only CLIENT, PRODUCER, and CONSUMER spans are projected. SERVER, INTERNAL, and
UNSPECIFIED spans return zero-valued endpoints, consistent with peer getters.

## Example

```go
endpoints := otlpmap.GetPeerEndpoints(span.Attributes(), span.Kind())
if endpoints.Target.Address != "" {
    // Configured database address, e.g. redis.prod.internal
}
if endpoints.Network.Address != "" {
    // Actual remote peer, e.g. 192.0.2.10
}
if endpoints.Network.HasPort {
    // Explicitly captured physical port: endpoints.Network.Port
}
```

For a Redis CLIENT span, the following OTel input:

```text
server.address       = redis.prod.internal
server.port          = 6379
network.peer.address = 192.0.2.10
network.peer.port    = 6380
```

returns `Target = (redis.prod.internal, 6379)` and
`Network = (192.0.2.10, 6380)`. Those endpoints intentionally differ. A
Cluster target such as `node1:6379,node2:6380` or a Sentinel target such as
`node1:26379,node2:26379/mymaster` is kept as **one opaque target string**,
with no attempted splitting into a single host and port.

## Evidence used for the initial test matrix

The implementation and test fixtures are based on source and expected values
from **OpenTelemetry Java instrumentation v2.32.0**:

- [Lettuce getter](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation/lettuce/lettuce-5.1/library/src/main/java/io/opentelemetry/instrumentation/lettuce/v5_1/LettuceDbAttributesGetter.java)
- [Lettuce integration test, testSetCommand](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation/lettuce/lettuce-5.1/testing/src/main/java/io/opentelemetry/instrumentation/lettuce/v5_1/AbstractLettuceSyncClientTest.java#L125-L160), asserting both server and network endpoint fields
- [Jedis getter](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation/jedis/jedis-4.0/javaagent/src/main/java/io/opentelemetry/javaagent/instrumentation/jedis/v4_0/JedisDbAttributesGetter.java)
- [Redisson getter](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation/redisson/redisson-common-3.0/javaagent/src/main/java/io/opentelemetry/javaagent/instrumentation/redisson/common/v3_0/RedissonDbAttributesGetter.java)
- [Redis logical target normalization](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation-api-incubator/src/main/java/io/opentelemetry/instrumentation/api/incubator/semconv/db/internal/RedisServerTarget.java)
- [Jedis Cluster/Sentinel target tests](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/v2.32.0/instrumentation/jedis/jedis-4.0/javaagent-unit-tests/src/test/java/io/opentelemetry/javaagent/instrumentation/jedis/v4_0/JedisServerTargetTest.java)

The Go test fixtures are **synthetic**, based on expected attribute contracts.
They are not raw OTLP exports from production and are not proof that production
Java Agents emitted all fields in the affected case. The actual first loss
point must still be verified end-to-end.

## Compatibility and downstream ownership

- Existing peer getters and flattened fingerprints remain byte-for-byte
  unchanged by this new opt-in API.
- The API provides **facts**, not an inferred deployment/entity identity.
  A Redis logical target and a Redis node address may legitimately differ.
- Downstream topology/CMDB integration decides whether to group by configured
  target, network endpoint, or an independently resolved resource ID.
- A caller that needs stable node identity should not treat `peer.db.system`
  or `GetPeerName` alone as a unique Redis instance ID.
- This does not automatically fix an existing topology pipeline that stores
  only `GetPeerTags`: storage, grouping and UI must consume endpoint facts.

## Validation checklist

- [x] Static source review of OTel Java Lettuce/Jedis/Redisson v2.32.0
- [x] Synthetic Go cases for canonical/legacy aliases, precedence, partial
      inputs, invalid port values, IPv6, Cluster, Sentinel and span kinds
- [x] Test for unchanged flattened peer tags and no input mutation
- [ ] Run Go 1.25 unit tests, race tests and allocation benchmark in a
      compatible environment; no test execution is implied by the fixtures
- [ ] Capture a sanitized **raw Redis CLIENT Span** from the affected deployment,
      with scope, schema URL, Agent/client versions and semconv stability mode
- [ ] Compare raw span → any migration → endpoint extraction → storage →
      grouping → topology UI; identify the first point at which fields disappear
- [ ] Validate the downstream identity contract and any storage schema migration
