# Hybrid Message Retention Design

## Context

`chatbox` 现在的房间消息留存主要依赖两层：

1. 本地加密 transcript
2. 在线客户端之间的 peer-to-peer history sync

这套模型在“多人不同时在线”的场景下存在明显短板：

- 某人离线时产生的新消息，只有当后来房间里有其他现代客户端在线且保留了本地 transcript，才有机会被补回来
- 如果消息发生在“所有潜在历史提供者都离线”的窗口，后上线客户端容易缺历史
- 文件已经有 host 端短期托管能力，但文字消息仍然过度依赖客户端同时在线

目标不是把 host 变成永久聊天数据库，而是提升多人错峰上线时的消息传递可靠性。

## Goal

新增一套“混合留存”机制：

- host 保存 `30 天` 的加密文字消息短期池
- host 继续保存 `7 天` 的附件
- 客户端继续保存长期本地 transcript
- 客户端上线后优先向 host 拉短期缺失历史
- 超出 host 保留窗口或 host 无法满足时，再回退到现有 peer-to-peer history sync

这样既能显著提升多人错峰上线时的消息可达性，又避免 host 成为永久消息中心。

## Non-Goals

- 不把 host 改造成永久 transcript 服务
- 不新增全文搜索
- 不新增已读/未读状态
- 不做服务端复杂 diff 索引
- 不做旧 transcript 自动迁移工具
- 不改变现有附件上传/下载模型
- 不改变实时在线消息的广播路径

## Retention Model

### Hybrid Retention

保留分两层：

1. Host 短期池
   - 文字消息：`30 天`
   - 撤回 tombstone：`30 天`
   - 附件：`7 天`

2. Client 长期池
   - 现有本地加密 transcript
   - 无固定期限，由本地文件决定

### Why Hybrid

相比“纯 host 留存”或“纯客户端增强同步”，混合模式的权衡更稳：

- host 短期池解决“大家不同时在线时消息传不过来”的核心问题
- 客户端长期 transcript 保留 30 天之外的老历史
- peer sync 退化为补充路径，而不是唯一离线恢复路径

## Security Model

### Encryption

host 端短期池必须加密落盘。

加密原则：

- 复用当前房间 PSK 作为根密钥材料
- 沿用现有 transcript/attachment 的对称加密思路
- 每条记录独立加密
- 每条记录使用独立 nonce

安全目标：

- 路由器磁盘文件不应直接暴露聊天明文
- 群聊密码不以明文形式额外持久化到消息池中

### Authorization Boundary

继续保留现有历史可见性原则：

- 某个 identity 只能拿到自己“首次加入该房间之后”的消息
- 知道群名和密码不等于可以直接拉取最近 30 天全量
- host 不能直接信任客户端上报的 `joined_at`

host 返回消息时必须同时满足：

- `record.room_key == request.room_key`
- `record.at >= host_authoritative_joined_at`
- `record.at >= now - 30 days`

### Host-Authoritative Join Timestamp

由于当前 `identity` 只有可导入导出的随机 ID，没有签名能力，host 无法证明客户端上报的更早 `joined_at` 真实可信。

因此 host 端 `30 天` 留存的授权下界必须改为：

- `host 首次看到该 identity 进入该 room 的时间`

也就是说：

- host short retention 只对“host 自己见过的加入时间”负责
- peer-to-peer sync 继续负责补更早、但仍属于同一 identity 的长期历史

这会带来一个明确边界：

- 某个 identity 第一次被这个 host 看到时，host 只能给它这之后的 30 天消息
- 如果该 identity 实际上更早就在别处加入过房间，更老历史仍然只能依赖 peer sync

这不是缺陷，而是当前“无签名 identity”模型下的安全边界。

## Data Model

host 端短期池不是 transcript 副本，而是独立的短期留存层。

### Stored Message Record

建议字段：

- `message_id`
- `room_key`
- `author_name`
- `author_identity`
- `body`
- `at`
- `ingested_at`
- `expires_at`

### Stored Revoke Record

建议字段：

- `target_message_id`
- `room_key`
- `operator_identity`
- `at`
- `ingested_at`
- `expires_at`

### Not Stored

- 附件二进制内容超过现有 7 天窗口
- 明文 transcript 文件
- 房间明文密码

## Revoke Semantics

如果消息后来被撤回，host 短期池必须保留撤回 tombstone。

行为要求：

- 后来上线的客户端只能看到“该消息已撤回”的结果
- 不能因为离线同步而重新看到原文
- 如果客户端本地之前已经有原文，再同步到 tombstone 时，应沿用现有 revoke 覆盖逻辑

这保证在线状态与离线补历史状态的可见性一致。

## Sync Architecture

### Existing State

当前离线恢复路径主要是：

1. client 连接房间
2. 发送 history sync hello
3. 在线 peer 根据摘要互相 offer/request/chunk

问题在于：这条链路依赖“当时有别的现代客户端在线”。

### New Layer

在现有 peer sync 之前新增一层 `host history sync`。

同步顺序改成：

1. 客户端连接成功
2. 继续发送现有：
   - `version announce`
   - `history sync hello`
3. 客户端向 host 发起 `host history request`
4. host 返回 `host history chunk`
5. 客户端先落 host 返回记录
6. 如果仍有缺口，再继续现有 peer sync
7. 两路结果统一去重、排序、持久化

### Why Host First

- host 在线概率最高
- host 短期池正好覆盖“最近消息错峰上线”的主要问题
- peer sync 更适合补 host 窗口之外的老历史

## Control Protocol

### New Hidden Control Family

建议新增：

- `\x00chatbox:hostsync:request`
- `\x00chatbox:hostsync:chunk`

要求：

- 复用现有 hidden control pattern
- 旧版本客户端不应把这些控制消息显示成普通聊天正文

### Host History Request

建议字段：

- `version`
- `room_key`
- `identity_id`
- `joined_at`
- `newest_local`

其中：

- `identity_id` 必须是该客户端长期持有、可导入导出的稳定身份 ID
- `joined_at` 只能作为客户端本地同步优化提示，不能作为 host 授权依据

首版不建议加入重量级 `known_message_ids` 集合，避免协议和载荷复杂度上升。

### Host History Chunk

建议字段：

- `version`
- `room_key`
- `target_identity`
- `records`
- `revokes`

### Sync Window Strategy

首版 host 不做复杂 diff，只按时间窗口返回：

- `record.at >= max(host_authoritative_joined_at, newest_local - drift_buffer)`

其中 `drift_buffer` 建议保留一个小缓冲窗口，例如 `2 分钟`，用于吸收：

- 端上时间漂移
- reconnect 边界
- 记录写入/读取竞态

客户端负责最终去重。

## Client Merge Rules

无论记录来自 host sync 还是 peer sync，都必须进入同一套合并路径。

统一规则：

- 优先按 `message_id` 去重
- 保留现有“等价消息模糊去重”能力
- 统一按消息时间排序插入
- 统一走 revoke 覆盖逻辑
- 统一落本地 transcript

不能为 host sync 和 peer sync 各自维护一套不同的插入逻辑，否则重复消息问题会再次出现。

## Compatibility

### New Client + New Host

完整启用：

- host sync
- peer sync
- 统一去重排序

### New Client + Old Host

行为要求：

- host 不认识 `hostsync`
- client 静默降级回现有 peer sync
- 不影响正常聊天

### Old Client + New Host

行为要求：

- old client 不发送 `hostsync request`
- 仍然正常收发实时消息
- host 新留存层不能影响旧客户端实时聊天

### Mixed-Version Room

原则：

- 实时消息广播永远优先
- host sync 是增强能力，不是硬依赖
- peer sync 在过渡期继续保留

## Host Storage Lifecycle

### Expiration

host 应定期清理过期记录：

- 文字消息和 revoke：`30 天`
- 附件：沿用现有 `7 天`

### Triggering

建议清理时机：

- host 启动时清理一次
- 运行中按固定周期清理

这与当前附件清理模型保持一致。

### Authorization Metadata Persistence

host 还必须持久化一份独立的 room authorization 元数据，用来记录：

- `room_key`
- `identity_id`
- `host_authoritative_joined_at`

要求：

- 以 `(room_key, identity_id)` 作为稳定键，不能使用进程内临时连接 ID
- 这份元数据必须跨 host 重启保留
- 不能只存在内存里
- host 重启后仍应继续使用先前记录的 `host_authoritative_joined_at`

否则：

- host 一重启就会“忘记”某个 identity 何时首次加入
- 进而破坏 `joined_at` 授权边界
- 还可能让离线补历史窗口在每次重启后漂移

因此，host 短期池和 host 授权元数据必须一起持久化。

## UI Expectations

首版 UI 只做轻量提示，不扩展复杂状态面板。

建议提示：

- `history sync: host`
- `history synced: N messages`
- `history sync failed`

如果 host sync 失败：

- 只记轻量 system 行
- 不阻断聊天
- 继续尝试 peer sync

如果 peer sync 失败：

- 同样只降级，不阻断聊天

## Implementation Scope

首版建议只做最小可用集合：

1. host 端 30 天加密文字消息池
2. hostsync hidden control codec
3. client 连接后先做 host sync
4. host sync 后按需继续 peer sync
5. revoke tombstone 进入 host 池
6. 统一 merge path

不在首版实现：

- 服务端复杂索引
- 服务端全文搜索
- 消息已读状态
- 管理端查看工具
- 历史迁移工具

## Testing Strategy

### Host Retention Layer

测试点：

- 消息加密落盘
- revoke tombstone 加密落盘
- 30 天过期清理
- 非文字消息不进入 30 天文字池

### Authorization

测试点：

- 只能返回 `host_authoritative_joined_at` 之后的消息
- 同房间不同 identity 返回窗口不同
- 同群名不同密码不会看到同一房间短期池

### Sync Flow

测试点：

- 新 client 连接新 host 时先走 host sync
- host 无数据时仍继续 peer sync
- old host 不支持时自动降级

### Merge / Dedupe / Ordering

测试点：

- host sync 与 peer sync 同时返回同一消息时只保留一份
- 时间戳轻微漂移时仍能等价去重
- 同步回来的历史插入正确时间位置
- revoke 在 host sync 与 peer sync 路径上表现一致

### Compatibility

测试点：

- old client + new host 正常聊天
- new client + old host 正常聊天
- hidden control 不会漏显到聊天窗口

## Recommended Rollout

建议按以下顺序落地：

1. 新增 host 短期池和加密存储
2. 新增 hostsync 协议
3. client 先接 host sync
4. 把 host sync 接入现有统一去重/排序路径
5. 最后补 UI 提示和回归测试

这样可以在不扰动实时聊天主路径的前提下，逐步增强离线消息可达性。
