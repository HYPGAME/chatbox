# Group Name And Password Room Design

## Context

当前 `chatbox` 的群聊本质上是：

- 一个 `host`
- 多个 `join`
- 共享同一个 32 字节 PSK
- 房间身份主要由地址决定

现状有两个问题：

1. 使用门槛偏高
   - 用户需要先准备 `--psk-file`
   - 分享和加入一个新群聊时不够直接

2. 房间身份不稳定
   - transcript、离线同步、撤回、房间授权等逻辑都依赖 `room key`
   - 当前 `room key` 由 `listen/peer` 地址生成
   - host 地址变化后，即使还是同一批人、同一个 PSK，也会被当成新房间

本次要新增一套“群聊名 + 密码”模式，降低创建/加入群聊的成本，并让群聊身份在地址变化时保持稳定。

## Goal

在保留现有 `host/join` 架构的前提下，新增一套基于“群聊名 + 密码”的房间模式。

目标：

- 用户可以不准备 `--psk-file`，直接通过群聊名和密码创建/加入群聊
- 同一组“群聊名 + 密码”在不同设备上派生出相同 PSK
- 群聊模式下使用稳定房间身份，而不是地址房间身份
- host 地址变化后，历史记录、离线同步、撤回、房间授权仍按同一个群聊继续工作
- 不破坏现有 `--psk-file` 模式

## Non-Goals

- 不做 host 自动发现
- 不做 mesh 或去中心化群聊
- 不新增 `group host` / `group join` 子命令
- 不支持环境变量密码
- 不支持 `--group-password-file`
- 不做旧 transcript 自动迁移
- 不做协议级“混用模式强制拦截”

## CLI Design

### New Flags

`host` 和 `join` 都新增：

- `--group-name`
- `--group-password`

### Mode Selection

两套模式互斥：

1. 传统模式
   - `--psk-file`

2. 群聊模式
   - `--group-name`
   - `--group-password` 或交互静默输入

禁止：

- `--psk-file` 与 `--group-name` 同时出现
- `--psk-file` 与 `--group-password` 同时出现

### Flag Rules

- `--group-name` 和密码必须成套使用
- `join` 仍然必须传 `--peer`
- `host` 仍然必须传 `--listen`
- `--group-name` 不能为空白字符串
- 群密码不能为空字符串

### Password Input Rules

优先级：

1. `--group-password`
2. 交互静默输入

行为：

- 如果提供了 `--group-name` 且显式传了 `--group-password`，直接使用
- 如果提供了 `--group-name` 但未传 `--group-password`：
  - 在交互终端中静默提示输入
  - 在非交互环境中直接报错，提示必须显式提供密码

### Examples

创建群聊：

```bash
chatbox host --listen 0.0.0.0:7331 --name alice --group-name team-alpha
```

加入群聊：

```bash
chatbox join --peer 10.77.1.4:7331 --name bob --group-name team-alpha
```

自动化或路由器场景：

```bash
chatbox host --listen 0.0.0.0:7331 --name router --group-name team-alpha --group-password abc123
```

## Derived PSK Design

群聊模式下不再从文件读取原始 PSK，而是从：

- `group-name`
- `group-password`

确定性派生出 32 字节 PSK。

### Normalization Rules

- `group-name`
  - `strings.TrimSpace`
  - 空字符串非法
- `group-password`
  - 保持原样
  - 不做大小写归一
  - 空字符串非法

### Derivation Strategy

使用 HKDF-SHA256 派生 32 字节 PSK。

输入要求：

- 使用固定域分隔字符串，例如 `chatbox group room psk`
- 把规范化后的 `group-name` 和原始密码一起作为派生输入

目标：

- 同样群聊名 + 同样密码 -> 同样 PSK
- 任一项不同 -> 不同 PSK
- 派生结果长度固定为 32 字节，与现有握手/附件加密要求一致

### Security Notes

这不是账户系统，也不是服务端凭据存储。

本质上仍然是共享口令模型：

- 群密码越弱，群越容易被猜中
- `chatbox` 不负责防爆破或限流
- 推荐用户使用高熵密码

## Stable Room Identity Design

这是本次设计的核心。

### Current Behavior

当前 room key 主要按地址生成：

- host：`host:<listenAddr>`
- join：`join:<peerAddr>`

因此地址变化会导致：

- transcript 视为新房间
- 离线同步房间不一致
- 撤回控制房间不一致
- 房间授权记录不一致

### New Group Room Key

群聊模式下使用稳定 room key。

为避免“同名但不同密码”的群在本地元数据层互相污染，room key 不能只包含群聊名，还需要包含从派生 PSK 得到的稳定指纹。

推荐格式：

```text
group:<normalized-group-name>:<psk-fingerprint>
```

其中：

- `normalized-group-name` = 去掉首尾空白后的群聊名
- `psk-fingerprint` = 从派生后 32 字节 PSK 计算出的短稳定指纹

指纹目标：

- 同样群聊名 + 同样密码 -> 同样 room key
- 同样群聊名 + 不同密码 -> 不同 room key
- 不暴露完整 PSK

### Effect

只要用户进入的是同一个群聊名，并且密码正确：

- 派生 PSK 一致
- room key 一致
- 历史记录继续复用
- 离线同步继续复用
- 房间授权继续复用
- 撤回控制继续复用

地址只表示“当前连到哪台 host”，不再表示“这是不是同一个群”。

同时：

- 同群改地址 -> 仍是同一个房间
- 同名但改密码 -> 视为不同房间

## Data Flow Impact

### Transcript

群聊模式下 transcript 文件按稳定 group room key 归档。

结果：

- 同群换地址后，仍然打开同一份 transcript
- display name 继续不是 transcript 分房间条件

### Room Authorization

房间授权加载和存储继续使用现有逻辑，但 room key 改为 group room key。

结果：

- `JoinedAt`
- identity-room 绑定
- 历史同步可见范围

都按群聊名稳定复用。

### History Sync

以下控制消息中的 `RoomKey` 字段，在群聊模式下都改为 stable group room key：

- `HistorySyncHello`
- `HistorySyncOffer`
- `HistorySyncRequest`
- `HistorySyncChunk`

结果：

- host 地址变化后，同群历史同步仍然能匹配
- 同群离线消息恢复不再依赖旧地址

### Revoke And Update Controls

以下控制消息继续复用现有结构，但 room key 切到 group room key：

- revoke
- `/update-all`
- update result
- events/status 相关房间授权链路

结果：

- 群聊模式下这些控制行为继续以“群”为边界，而不是以地址为边界

### Attachments

附件服务地址仍然来自当前 host 地址。

群聊模式下：

- 附件加密继续使用派生后的 PSK
- 附件下载目标仍然取决于当前连接到的 host
- 是否属于“同一个群”由 stable room key 决定

这两者不冲突。

## Compatibility

### Traditional PSK Mode

`--psk-file` 模式完全保持不变：

- 握手逻辑不变
- transcript 逻辑不变
- 路由器部署不变
- 文档中的旧用法继续有效

### Protocol Compatibility

本次是 CLI 和房间标识层扩展，不要求升级传输协议版本。

原因：

- 握手仍然只依赖最终 32 字节 PSK
- 消息格式不需要新增字段
- 控制消息结构不需要新增字段，只是 room key 的取值改变

### Mixed Mode Caveat

这是一个必须明确的限制：

如果同一个实时群里混用了两类现代客户端：

1. 新群聊模式：`--group-name`
2. 旧文件模式：`--psk-file`

并且双方底层 PSK 恰好相同，那么：

- 实时消息可能仍然连得上
- 但基于 room key 的能力会分叉：
  - transcript
  - history sync
  - revoke
  - room authorization

原因是：

- 群聊模式 room key = `group:<name>:<psk-fingerprint>`
- 传统模式 room key 仍然是地址模式

第一版不做协议级探测或强制拦截。

产品约束明确为：

- 同一个群一旦采用群聊模式，所有现代客户端都应使用群聊模式加入
- 混用 `--psk-file` 与群聊模式属于不受支持场景

该限制需要写入 README 和使用提示。

## Error Handling

需要覆盖以下错误：

- `--psk-file` 与群聊参数同时出现
- 只提供 `--group-name` 但密码缺失，且当前环境非交互
- `--group-name` 为空白
- `--group-password` 为空
- 静默输入失败
- 群密码错误导致握手失败

错误展示原则：

- 参数错误：在 CLI 参数解析阶段直接报错
- 密码错误：保持现有 PSK 握手失败语义，不额外暴露内部派生细节

## Testing

### CLI And Input Tests

- `host` 与 `join` 支持群聊模式参数
- `--psk-file` 与群聊参数互斥时报错
- 缺少群密码时：
  - 交互环境触发静默输入
  - 非交互环境报错
- 空白群名、空密码报错

### PSK Derivation Tests

- 同样群聊名 + 同样密码 -> 相同 32 字节 PSK
- 群聊名变化 -> PSK 变化
- 密码变化 -> PSK 变化
- 派生长度始终为 32 字节

### Room Key Tests

- 群聊模式下 host/join 都使用同一个 `group:<name>:<psk-fingerprint>` room key
- 传统模式仍使用现有地址 room key
- 更换地址不会改变群聊模式 room key
- 仅修改密码会改变群聊模式 room key

### Transcript And Authorization Tests

- 群聊模式下 transcript 文件稳定复用
- 群聊模式下 room authorization 使用 group room key
- 传统模式回归不受影响

### Integration Tests

- `host` 和 `join` 用同一群聊名 + 密码可以连接
- 密码错误握手失败
- 更换 `peer` 地址后，同群 transcript 继续加载
- history sync 在群聊模式下使用 group room key
- revoke 在群聊模式下使用 group room key

## Future Work

如果后续要继续发展这一方向，可再考虑：

- `--group-password-file`
- 环境变量密码输入
- 群聊分享码
- host 自动发现
- 混用模式探测与用户提示
- 旧地址 room transcript 向 group room transcript 的迁移工具
