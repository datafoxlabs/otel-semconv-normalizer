# Resource / Peer 基数与 PII 落库策略

**状态：** 已采纳（Story 5.2）<br>
**适用范围：** `otlpmap` 调用方、Collector/exporter adapter 与 ClickHouse schema 设计<br>
**最后更新：** 2026-07-27

## 1. 结论

`GetResourceName`、`GetPeer*`、`GetPeerName` 与 `_df.flattened.peer.tags` 返回的是逐 span
候选值或原始事实，不是“可直接聚合”的类型证明。落库边界必须把两类数据分开：

- **归一维度**：已证明有界，服务于热查询、`GROUP BY` 和聚合表；
- **受控原始列**：服务于排障；写入前已经 scrub/redact，并按采样、TTL 和权限治理。

采样、TTL、RBAC 只能缩小暴露面，不能替代 PII 脱敏。包含 URL userinfo/query secret、SQL
字面量或其他敏感信息的值，在**任何持久化前**都必须清洗；无法确认安全时应 drop，而不是仅转存到
“raw”列。

发行版契约要求 OTel Agent 在 export 前完成 DB query 与 peer IP/PII 脱敏。本文中的“原始事实”
表示 otlpmap 不再改写 Agent 输出，不表示未经脱敏的原始载荷。otlpmap 不调用
`QuantizePeerIPAddresses`，也不生成 `blocked-ip-address`；不满足发行版契约的 producer 必须在
进入本库前完成等价清洗或被拒绝。

`semconvweave` 可以在持久化 projection 中把 legacy **key/name** 改为 canonical，但不会
执行降基数、脱敏或原地修改输入 Map。本文的“增列不改列”约束 value 重塑、candidate 和聚合维度，
不禁止经评审的 SemConv key rename。完整边界见
[Schema 归一化设计](weaver-build-contract.md)。

```mermaid
flowchart LR
    I["Instrumentation<br/>优先产生语义模板"] --> O["OTel span 属性"]
    O --> A["发行版 OTel Agent<br/>DB query 与 IP/PII 脱敏"]
    A --> M["otlpmap<br/>确定性逐 span 投影"]
    M --> C["候选 resource / 原始 peer 事实"]
    C --> S{"已 scrub 且<br/>基数契约可信？"}
    S -->|"是"| N["有界归一列<br/>resource_normalized / peer_group"]
    S -->|"只适合排障"| R["受控原始列<br/>采样 / TTL / RBAC"]
    S -->|"否或含未清洗 PII"| D["drop / bounded sentinel"]
    N --> H["ClickHouse 热聚合"]
    R --> Q["按需 drill-down"]
```

职责边界：

1. instrumentation/framework 最适合生成 `http.route`、`url.template`、DB summary 等带业务语义的模板；
2. 发行版 OTel Agent 在 export 前完成 DB query 与 peer IP/PII 脱敏；
3. `otlpmap` 保持无状态、无 I/O；现有 getter 做兼容读取和确定性选择，规划中的静态 weave 为
   调用方 projection 提供 key/name/value 决策，不重复扫描或原地改写 peer tag；
4. Collector/exporter 或等价落库 adapter 验证 producer 契约，并负责准入、映射和有界降级；
5. ClickHouse 负责保存已经治理过的列，不用存储类型替代上游治理。

本策略不会在逐 span 的核心热路径引入通用 Schema Processor、正则模板猜测、DNS/CMDB
查询或完整属性 Map 重写。

## 2. 术语与硬约束

| 术语 | 含义 |
| --- | --- |
| candidate | getter 按当前优先级返回的逐 span 候选值；可能仍含 ID、PII 或动态租户名 |
| normalized dimension | 经过准入/映射后，distinct count 可被部署方观测和约束的聚合列 |
| controlled raw | 已完成必要脱敏、仅用于排障，并配置采样、TTL 和访问控制的原始事实列 |
| bounded sentinel | 部署方配置的有限状态值，例如“未知/未模板化”；具体拼写不是本库 API |

必须同时满足以下约束：

- **长度不是基数。** `maxResourceNameLen = 5000` 只限制单值 byte length，不能降低 distinct
  count；既有按字节截断还可能切断 UTF-8 code point。
- **字段名不是可信证明。** `http.route`、`url.template`、`db.query.text`、`db.query.summary`、
  `resource.name`、`peer.service` 等字段都可能被错误 instrumentation 写入动态值。
- **hash 不是降基数。** 对 raw path、peer fingerprint 或 broker list 求 hash 只改变宽度，不改变
  distinct count。
- **value 重塑增列不改列。** 降基数、PII 处理和候选聚合结果写入独立列，不覆盖
  `resource`、`peer.*` 或输入 OTel value。SemConv legacy key→canonical key projection 由独立
  Schema 设计管理。
- **fail closed。** 缺少可信模板/映射，或清洗器失败时，聚合列降级为 method-only、空值或有界
  sentinel；不得回退到未经验证的 raw 值。

Epic 7 的 attribute projection 只在写 CK attributes JSON 时消费 decision，不修改 source
`pcommon.Map`，也不参与 `GetResourceName`。因此 SemConv weave 不改变 `resource_name` 候选值；
它继续使用 package-level getter 的 non-empty fallback。PII、准入、容量阈值和是否 promotion
仍由 Collector/persistence owner 决定。

## 3. Resource 来源与动作矩阵

当前 `GetResourceName` 的候选值仍按代码优先级产生；下表定义的是调用方在持久化边界上的第二次判定。

| 来源 | 当前候选语义 | 主要风险 | 归一列动作 | 原始列动作 |
| --- | --- | --- | --- | --- |
| `resource.name` | 项目/vendor override，最高优先级 | 可绕过全部派生；可能含 ID/PII | 只接受受治理 producer、allowlist 或显式映射；否则 sentinel/drop | 先 scrub/redact；无法确认安全则 drop |
| HTTP server | method + `http.route` | producer 可能把具体 path 写进 route；自定义 method 也可能动态 | method 先收敛到受控词汇，再校验模板；不可信时 method-only/sentinel | 所有 URL 组件完成敏感信息分类与 scrub 后才可保存 |
| HTTP client | method + `url.template` | 模板字段可能被误写 | method 先收敛；只接受可信 template | 同上 |
| HTTP method + 其他 kind | 当前实现只返回 method candidate，不读取 target | 非标准 kind、自定义 method | 不视为 server/client 模板；只接受有界 method，否则 sentinel | 关联 URL 仍按 raw 策略处理 |
| messaging | operation name + destination | 动态 topic/queue、租户后缀 | allowlist、显式 destination mapping 或 sentinel | scrub 后受控保存 |
| RPC | method 或 `method service` | 动态 service/method、自定义插桩 | 校验已知 service/method 词汇；否则 sentinel | scrub 后受控保存 |
| GraphQL | operation type + operation name | 客户端生成名、匿名请求、动态值 | 只接受受治理 operation name；否则 type-only/sentinel | query/document 不由 resource 列代替保存 |
| DB query（仅 DB system 非空） | Agent 已清洗的 `db.query.text → db.statement` | Agent 配置退化时可能泄露字面量；动态 identifier 仍可能高基数 | 发行版 Agent 预清洗是接入硬门禁；聚合仍需 distinct 监控 | 可作为 DD Resource；无法证明已清洗时 drop |
| `fallbackName` | 调用方固定传 `span.Name()` | 可能含 `/orders/42`、SQL 或用户自定义值 | 默认不信任；显式准入后使用，否则 sentinel/drop | 先 scrub/redact；无法确认安全则 drop |

DB query 进入 resource 的安全依据是发行版 Agent 的 query sanitization 契约，而不是字段名本身。
`resource.name`、messaging/RPC/GraphQL 与 fallback 仍分别按自身来源治理。

## 4. HTTP 模板规则

OpenTelemetry HTTP 语义约定只允许在 target 低基数时把它放入 span name：server target 使用
`http.route`，client target 使用 `url.template`。规范同时明确不能默认用 URI path 充当 target。
本项目与 DD 一致：server 读取 `http.route`，client 只读取 `url.template`。

```mermaid
flowchart TD
    H["HTTP resource candidate"] --> K{"kind"}
    K -->|"server"| SR["读取 http.route"]
    K -->|"client"| CT["读取 url.template"]
    K -->|"其它"| OT["仅 method candidate"]
    SR --> V{"模板契约可信？"}
    CT --> V
    OT --> B
    V -->|"是"| T["METHOD + template"]
    V -->|"否/缺失"| B["METHOD-only 或 bounded sentinel"]
    H --> RAW["url.full / url.path / legacy http.target"]
    RAW --> X["仅经 scrub 后进入受控原始列<br/>绝不反推 route"]
```

落地要求：

- 不从 `url.path`、`url.full`、旧 `http.target` 或 span name 用正则猜 `{id}`；
- `/orders/42` 即使出现在 `http.route`/`url.template` 中，也应视为不可信模板并计数；
- method-only 也必须有界：只允许部署方批准的标准/自定义 method；未知原值映射为 `_OTHER` 一类
  bounded token 或 sentinel，不把 `method_original` 直接放入聚合列；
- 缺少模板时，选择有界 method-only、空值或 bounded sentinel；部署内只选一种并固定语义；
- URL userinfo 必须删除；scheme、host/subdomain、path、query 和 fragment 都要做敏感信息分类，
  其中 token、password、API key、邮箱、用户名、租户/对象 ID 等必须 redact/drop；
- raw URL/path 如确需保存，只能在所有组件完成上述清洗后进入受控原始列。

## 5. DB Resource 与 Agent 侧清洗契约

只有 `db.system.name → db.system → db.type` 任一非空时，`GetResourceName` 才进入 DB
分支，顺序为 `db.query.text → db.statement → span.Name()`。`GetDBQueryText` 使用相同的
current→legacy query 顺序。

```mermaid
flowchart TD
    D{"DB system 非空？"} -->|"否"| F["进入普通 fallback 治理"]
    D -->|"是"| Q{"已清洗 query 非空？"}
    Q -->|"是"| R["作为 DD Resource candidate"]
    Q -->|"否"| F["fallbackName = span.Name()"]
    A["发行版 OTel Agent"] --> S["query sanitization"]
    S --> Q
```

发行版与接入 adapter 必须共同保证：

- 所有允许接入的 OTel Java Agent 构建均强制开启 query sanitization，用户配置不能关闭；
- 发布前用每个已启用 DB system 的正反例语料验证字符串、数字、Redis 参数等被替换；
- 新增 producer 或 instrumentation 时先通过同一契约测试，未通过则禁止进入 Resource 持久化；
- 监控 query sanitizer 版本与 Agent 发行版，SQL 文本不得进入 metric label 或宽权限日志；
- Collector 不再重复解析 SQL，避免在 调用方目标吞吐量 热路径引入第二套 parser。

官方 DB Migration Guide 对 `db.statement` → `db.query.text` 附带 sanitization 前置条件，因此
Schema 中出现 rename 不能单独证明 value 已安全。已实现的 `semconvweave` 不会自动把
任意 `db.statement` 伪造成 `db.query.text`；优先要求 Agent 直接发出已清洗的新属性。getter 对
legacy key 的 fallback 仍受上述发行版准入契约约束。

## 6. Peer 原始事实与聚合身份

当前 14 个 `peer.*` 概念（含 Network 补齐的 `peer.port`）是逐 span 原始事实。
系统类型通常较稳定，但身份字段仍需按 tenant/service/window 实测，不能仅凭字段名判定低基数。

| 输出 | 实际语义 | 聚合策略 |
| --- | --- | --- |
| `peer.hostname` | 原始 IP/hostname，可能暴露个人地址或内部拓扑 | otlpmap 原值不改；持久化前按安全策略 redact/tokenize/drop，另增 CIDR/service mapping 列 |
| `peer.kafka.bootstrap.servers` | broker 集合，顺序也可能变化 | 不作 cluster identity；优先稳定 cluster ID 或显式映射 |
| `peer.rocketmq.broker` | 单 broker 地址 | 不把单 broker 直接当 RocketMQ 集群 |
| `_df.flattened.peer.tags` | 所有 present peer facts 的组合指纹，也会复制其中的 PII | 未确认每个组成值已 scrub 时不得持久化；不作低基数实体键 |
| `GetPeerName` | 按域优先级选择的显示名 | 可能选中 IP、broker 或 destination，不是归一器 |
| `peer.*.system` | canonicalized 技术类型 | 只允许受控 system 词汇；未知自定义值进入 raw/sentinel，不直接聚合 |
| service/destination/DB/AWS identity | producer 给出的身份 | 通过 allowlist、服务目录或显式 mapping 后进入 `peer_group` |

“增列不改列”的参考动作：

- `peer.hostname=10.1.2.3` 保持不变，部署方可另产 `peer_group=10.1.2.0/<configured-prefix>`；
- CIDR prefix 必须由网络拓扑 owner 配置，不在本库写死 IPv4/IPv6 默认值；
- hostname 优先映射到明确 service/cluster identity，不通过截断 label 猜服务；
- hostname/IP 在部分部署中属于个人数据或内部拓扑；“增列不改列”约束的是 otlpmap 输出，
  不代表存储必须保留明文，raw 列仍可按安全策略 redact/tokenize/drop；
- 缺少可信映射时使用空值或有界 sentinel，不能把 raw address 偷渡到 `peer_group`；
- Kafka 优先未来进入支持窗口的 `messaging.kafka.cluster.id` 或现有显式集群映射；
- broker list 排序/格式变化不应制造新的聚合实体；
- DynamoDB batch 的 getter 只返回首个非空 table，不能据此把多 table batch 聚合为单一对端；
- 原始 `_df.flattened.peer.tags` 在任何组成值未 scrub 时必须 drop。若需要 scrubbed fingerprint，
  应从已治理的独立列重新构造并使用新列名，不覆盖原始指纹；
- 对 fingerprint 求 hash 只适合缩短存储，不解决实体基数或 PII。

## 7. ClickHouse 参考布局

下面只表达列职责，不是本项目交付的生产 DDL，列名也不是 `otlpmap` API：

```sql
-- Conceptual example only. Engine, ORDER BY, TTL and codecs require local testing.
resource_normalized LowCardinality(String), -- 经准入且基数受控后才采用
resource_raw        String,                 -- 已 scrub 的受控排障值
peer_group          LowCardinality(String), -- 经 service/cluster/CIDR mapping
peer_hostname_raw   String,                 -- 经安全策略处理的 hostname/IP 排障值
peer_fingerprint_raw String,                -- 已 scrub 的逐 span 指纹，不作默认 GROUP BY
resource_source     LowCardinality(String), -- 可选：有界 source 枚举
policy_outcome      LowCardinality(String), -- 可选：accepted/sentinel/dropped/...
policy_version      LowCardinality(String)  -- 可选：支持审计与回滚
```

约束：

- `LowCardinality(String)` 是字典编码，不会自动减少 distinct count；只有实测基数与查询收益合适时才用；
- ClickHouse 官方文档给出的 distinct 数量只是经验区间，不能直接抄成所有 tenant 的 SLO；
- raw resource、URL、address、broker、SQL 和 fingerprint 不进入 `PARTITION BY`，也不作为
  `ORDER BY` 的前导字段；
- 高频 dashboard 优先读 normalized 列；是否建立聚合表/物化视图，须压测额外写放大；
- bloom filter 只在确有 raw 检索模式且实测收益覆盖写入/存储成本时评估；
- raw 列的 TTL、采样率、RBAC 和审计由部署方定义；无论如何都不得保存未清洗 PII。
- 除 tenant/service 窗口指标外，还要观测整表、partition/part 的 distinct、字典 bytes、压缩比和
  查询内存；局部 tenant 达标不代表共享字典/part 一定安全。

## 8. 上线观测与回滚

所有观测至少按 tenant、service 和固定时间窗分组，并补充整表、partition/part 视角。窗口长度和
阈值由容量 owner 根据真实流量配置。

落库 adapter 必须为每次决策产生有界元数据或等价低基数 counter：

- `source`：override、http-server、http-client、http-other、messaging、rpc、graphql、db-query、
  fallback；
- `outcome`：accepted、sentinel、candidate-dropped、raw-sampled-out、raw-dropped、scrub-failed、
  unmapped；
- `reason`：部署方审核过的有限枚举，不接受异常文本或原始值；
- `policy_version`：能够关联发布、shadow 对比和回滚。

这些信息可作为 adapter metrics，也可按查询需要选配为低基数列，不要求全部逐 span 持久化。
本文的 **drop** 只表示丢弃 candidate/目标列，不丢整条 span；`raw-sampled-out` 与
`scrub-failed/raw-dropped` 必须分开计数。

| 信号 | 目的 |
| --- | --- |
| `distinct(resource_normalized)` / `distinct(peer_group)` | 发现归一维度失控 |
| raw distinct / normalized distinct | 衡量模板/映射收敛效果 |
| empty/sentinel/drop ratio | 发现 fail-closed 过多或 instrumentation 退化 |
| override/fallback/DB query miss ratio | 找出绕过标准模板或缺少 DB query 的主要来源 |
| HTTP template accepted/rejected ratio | 发现 route/template 被写成具体 path |
| Agent query sanitization 契约测试与发行版覆盖率 | 发现 Agent 配置/版本退化 |
| peer IP/cluster mapping coverage | 判断 `peer_group` 是否足以支撑聚合 |
| normalized top values（限量） | 发现映射失控；只能使用已准入/脱敏值 |
| CK insert throughput/p99、压缩比、parts、字典 bytes、查询内存 | 验证 schema 选择没有破坏 调用方目标吞吐量 容量目标 |

监控系统不得接收 raw URL、SQL、address、broker、fingerprint 或自由格式 error 作为 label；需要诊断
样本时只能使用已经 scrub 的限量事件，并服从与受控 raw 列相同或更严格的权限与 TTL。

推荐发布顺序：

1. shadow 计算 normalized 列，不切换 dashboard；
2. 对比 distinct、sentinel/drop、PII scrub failure 和 ClickHouse 资源开销；
3. 达到部署方门槛后，让热查询切到 normalized 列；
4. 再启用 raw 列的受控保留策略；
5. 异常时回滚 normalized 规则或切到有界 sentinel，**不得回退用 raw 值聚合或保存未清洗数据**。

每次上线必须指定 instrumentation、adapter、ClickHouse 与安全 owner，并记录阈值、回滚开关及审计位置。

## 9. 验收矩阵

| 场景 | 归一列 | 受控原始列 |
| --- | --- | --- |
| server `GET /orders/42`，`http.route=/orders/{id}` | `GET /orders/{id}` | scrub 后可选保存 raw URL |
| client 只有 `url.path=/orders/42` | method-only/空/sentinel | scrub 后可选保存 path |
| `http.route=/orders/42` | 拒绝具体 path，method-only/sentinel | 作为不可信 raw 处理 |
| `url.full=https://user:pass@host/x?token=secret` | 不进入 | 必须移除 userinfo、redact query；失败则 drop |
| `resource.name=GET /tenant/42/orders/7` | 未经显式准入不得使用 | scrub 后受控保存；失败则 drop |
| `fallbackName=GET /orders/42` | 不盲用 fallback | scrub 后受控保存；失败则 drop |
| Agent 已清洗 DB query，DB system 非空 | query 作为 DD Resource candidate | 可按受控原始列策略保存 |
| DB query 缺失，DB system 非空 | 回退 `span.Name()` | 无 query 原始列 |
| Agent sanitization 契约测试失败 | producer 禁止接入 Resource 链路 | query/resource drop，不回退未清洗 statement |
| `peer.hostname=10.1.2.3` | 由部署配置映射到独立 `peer_group` | 按 IP/拓扑安全策略 redact/tokenize/drop |
| `peer.hostname=node-tenant42.internal`，无可信映射 | sentinel/drop，不截断 hostname 猜服务 | 按内部拓扑/PII 策略 redact/tokenize/drop |
| DynamoDB batch 含多个 table | 不用 getter 的首个 table 代表整个 batch | 完整批次只按受控 raw 策略保存 |
| 未知自定义 `peer.*.system` | allowlist 未命中则 sentinel | 原值按受控 raw 策略保存 |
| Kafka brokers 调序 | 不把列表当稳定 cluster identity | broker list 只作排障 |
| RocketMQ 单 broker | 不把 broker 当集群 | broker 地址只作排障 |
| `_df.flattened.peer.tags` 或其 hash | 不作低基数实体键 | 组成值未全部 scrub 时 drop；scrubbed 派生值另列 |
| 5000-byte clamp 后不是有效 UTF-8 | sentinel/drop，不把无效串用于聚合 | 存储要求 UTF-8 时 drop；本库 clamp 契约不变 |

## 10. Out of Scope

- 在 Collector 核心增加 SQL parser/obfuscation hook；
- 修改 `GetPeerName`、`SetPeerTags` 或其他现有优先级；
- 新增 getter、normalizer、`Attrs`/options/config 或可变全局注册表；
- 在核心库内解析 URI/SQL、猜 route、查询 DNS/CMDB、硬编码 CIDR 或改写 raw 属性；
- 交付 ClickHouse migration、物化视图、TTL、RBAC、采样率、告警阈值或运行时 metrics SDK；
- 提前接入尚未进入本项目 canonical 支持窗口的 `messaging.kafka.cluster.id`。

## 11. 参考

- [OpenTelemetry HTTP spans semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/)
- [OpenTelemetry DB migration guide v1.41.0](https://github.com/open-telemetry/semantic-conventions/blob/v1.41.0/docs/non-normative/db-migration.md)
- [OpenTelemetry URL sensitive-information guidance](https://opentelemetry.io/docs/specs/semconv/url/)
- [ClickHouse `LowCardinality` data type](https://clickhouse.com/docs/sql-reference/data-types/lowcardinality)
- [ClickHouse primary-key selection](https://clickhouse.com/docs/best-practices/choosing-a-primary-key)
