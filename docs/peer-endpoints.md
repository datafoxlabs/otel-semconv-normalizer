# Peer 端点与 Network 身份补齐

状态：未发布。适用对象：`Client`、`Producer`、`Consumer` Span。

本合同扩展原有 13 个 peer concepts，新增本项目字段 `peer.port`。规则统一在
normalizer 中执行；调用方消费最终 peer tags，无需再次选择地址。显示名仍由
`GetPeerName` 按既有领域优先级选择，显示名不是完整身份。

## 身份规则

1. 先按原有规则提取全部 peer concepts。
2. 存在下面任一逻辑目标时，直接保留原结果，不追加 Network 地址或端口：
   `peer.service`、`peer.hostname`、`peer.kafka.bootstrap.servers`、
   `peer.rocketmq.broker`、`peer.aws.dynamodb.table`、`peer.aws.s3.bucket`、
   `peer.aws.kinesis.stream`。
3. 没有上述目标，但有 Network 地址时，以原值补齐 `peer.hostname`；若有同一
   Network 端点的有效端口，再写 `peer.port`，格式为十进制字符串。
4. 没有地址时保持原结果。仅有端口不形成身份，不补默认端口。

数据库类型、库名、topic、RPC service 名本身不表示部署位置，因此不阻止地址补齐。
已有 DB/MQ/RPC 类型和业务标识全部保留。上述目标指按下表及既有 peer source 映射
从原始 Span 提取的结果；接口输入仍是原始 typed OTel attributes，不是只有落库
`peer.*` 的投影。`peer.service` 是保留支持的显式输入。

配置中的 `ip:port,ip:port`、Redis Cluster/Sentinel 字符串作为一个整体原样保留：
不拆节点、不取第一个地址、不排序地址列表，也不把某个 Network 端口附到整个列表。
这不是集群识别算法。只有 Network 地址时，得到的是连接端点身份；IP 变化可以产生
不同节点，无法从一个连接 IP 还原缺失的集群配置。

## Endpoint 提取合同

`GetPeerEndpoints(attrs, kind)` 只读返回 `PeerEndpoints{Target, Network}`。
每个端点都有 `Address string`、`Port uint16`、`HasPort bool`。

| 返回字段 | 来源优先级 |
|---|---|
| Target.Address | `server.address` → `net.peer.name` |
| Target.Port | `server.port` → `net.peer.port` |
| Network.Address | `network.peer.address` → `net.sock.peer.addr` → `net.peer.ip` |
| Network.Port | `network.peer.port` → `net.sock.peer.port` |

地址只接受字符串且不改写。端口只接受 `pcommon.ValueTypeInt` 的 `1..65535`；
缺失或无效端口返回 `HasPort=false`，可用地址仍保留。端点 API 允许单独观察端口，
但 peer 身份输出要求有地址。提取函数不抛错、不修改输入、不记录原始属性日志。

Target 和 Network 不混用：例如 `net.peer.ip + net.peer.port` 不能伪造 Network
地址端口对。当前属性与旧别名地址明确冲突时，不使用属于旧地址的端口。
端点提取与身份选择共享同一份 Network 解析逻辑：
`GetPeerEndpoints` 返回端点事实，`GetPeerTags` 在逻辑目标缺失时将它纳入身份。

OTel 字段语义参考：[Network 属性](https://opentelemetry.io/docs/specs/semconv/registry/attributes/network/)、
[数据库 Span](https://opentelemetry.io/docs/specs/semconv/db/database-spans/)。保留逻辑目标优先、
以及 `peer.port` 的输出是本项目合同，不声称是 OTel 或 Datadog 统一规定。

## 输入输出示例

| 原始 Span 的相关字段 | 最终 peer 属性 |
|---|---|
| `db.system=redis`，Network=`192.0.2.10:6379` | `peer.db.system=redis`，`peer.hostname=192.0.2.10`，`peer.port="6379"` |
| `db.system=mysql`，`db.namespace=orders`，Network=`192.0.2.11:3306` | 保留类型和库名，补 hostname 与 port；另一地址上的同名库是另一身份 |
| `server.address=cache.internal`，Network=`192.0.2.10:6379` | `peer.hostname=cache.internal`；Network 不进入身份 |
| `server.address=node2:6380,node1:6379`，Network=`192.0.2.10:6379` | `peer.hostname=node2:6380,node1:6379`，保持顺序和完整值 |
| Kafka topic=`orders`，bootstrap=`b:9092,a:9092`，有 Network 地址 | 保留 system、topic、bootstrap；不增加 Network 身份 |
| Kafka topic=`orders`，无 bootstrap/hostname，有 Network 地址 | system、topic 加连接端点；不声称推导出 Kafka cluster |
| 仅 `db.system=redis` | 仍是 `peer.db.system=redis`，不猜测 hostname 或 port |

例如第一行最终由调用方编码的完整 peer JSON 应包含：

```json
{"peer.db.system":"redis","peer.hostname":"192.0.2.10","peer.port":"6379"}
```

`GetPeerHostname` 与 `GetPeerPort` 遵循同一规则。`GetPeerName` 的排序不变：MySQL
例子的名称仍可为 `orders`，完整身份则包含端点。`Range`、`RangePeerAttributes`、
`AppendFlattened`、`GetFlattenedPeerTags`、`SetPeerTags` 使用同一份 peer 结果。
`SetPeerTags` 把 port 写成字符串；如果原有 Network port 不再属于当前投影，会删除
陈旧 `peer.port`，避免它继续进入调用方的完整 peer key。读取函数不修改源 Span。

## 兼容、成本与验证边界

- flattened 存储字段统一为 `_df.flattened.peer.tags`。之前公开快照的
  `otelmap.flattened.peer.tags` 不再由本库生成；调用方需更新字段映射。不双写、不自动
  迁移历史数据；源 Span 已带有的旧扩展属性保持原值，不作为新指纹读取。
- 已有逻辑目标的 peer 输出和 flattened 内容保持不变；新增字段不改变原有字段顺序。
- Network-only 输入的 `peer_key` 会改变，包括 Redis 以外此前缺少地址的依赖；新旧历史
  身份不会自动合并。下游升级应作为一次可观察的身份规则变更。
- 逻辑目标分支保留原有不分配内存的读取路径。Network 补齐分支增加端点读取；把整数
  port 转成字符串可能分配内存。不引入端口缓存或复杂零分配结构。
- `peer_key` 仍由调用方对全部最终 `peer.*` 编码。本库不生成拓扑、不修改表结构，
  不把 Network IP 推断为集群名或实体模型 ID。
- `peer_endpoint_test.go` 覆盖端点别名、冲突、复合地址和 kind；
  `peer_network_test.go` 覆盖跨类型补齐、原身份稳定、完整字符串输出、地址/端口区分、
  有效端口边界、只读快照和写回幂等。用例为构造输入，不是线上真实流量。
- 本轮不声称完成调用方落库、ClickHouse 刷新、UI 或吞吐验收；后续接入时验证
  原始 Span → 最终 peer 属性 → `peer_key` → 拓扑/时序查询。

## Endpoint 规则的上游证据

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
