# TUI Visual Upgrade Design

## Context

当前 chatbox 的默认体验是 `scrollback`，但用户明确希望优先提升 `--ui tui` 的观感。现有 TUI 已经具备一些聊天软件式能力：

- 消息列表、输入框、状态栏
- 复制、引用、撤回模式
- 鼠标选择消息和操作按钮
- 附件打开、下载、进度状态
- 回复草稿和回复卡片渲染
- 历史同步、撤回、附件等系统状态

现在的问题不是缺少完整群系统，而是 TUI 的视觉层级还偏“工程调试界面”：状态信息、消息内容、系统提示、附件和回复都在同一视觉密度里，长时间聊天时不够清爽。

## Goal

做一版低风险的 TUI 视觉升级，让它更像一个认真打磨过的终端聊天软件。

目标：

- 打开 TUI 后第一眼更清楚：在哪个房间、是否在线、多少人在线
- 聊天区更容易扫读：日期、系统消息、连续发言、普通消息有明确层级
- 回复和附件更像聊天内容，而不是命令输出
- 底部输入区把“正在回复谁”“状态提示”“可输入内容”分清楚
- 不改变协议、历史存储、消息同步和 scrollback 行为

## Non-Goals

- 不做右侧成员栏
- 不做多房间列表
- 不做头像、气泡、复杂主题系统
- 不做 QQ 群式群管理
- 不改网络协议、transcript 格式、附件格式
- 不把 TUI 重写成 Web UI 或桌面 UI
- 不改默认 `scrollback` 模式

## Chosen Direction

选择“美化现有 TUI”的路线，而不是重做大布局。

第一版只改 TUI 渲染和少量布局计算：

1. 顶部房间状态栏
2. 消息区视觉整理
3. 日期分割和系统消息弱化
4. 回复卡片继续优化并纳入整体样式
5. 附件卡片样式
6. 底部输入区分层

这条路线保留现有交互模型，兼容已有测试和终端环境，复杂度明显低于三栏聊天 UI。

## Layout

TUI 保持三段式结构：

```text
top bar
message viewport
input area
```

### Top Bar

顶部栏固定一行，展示高价值状态：

```text
chatbox · team-alpha · connected · 3 online
```

字段来源：

- `chatbox`：固定品牌标识
- 房间/模式：优先显示 group name；没有 group name 时显示 host/join 模式或 peer 摘要
- 连接状态：复用现有状态，例如 connected、connecting、offline
- 在线人数：复用 `/status` 已经维护的 roster 信息；不可得时隐藏

窄终端降级：

- 宽度足够时显示完整信息
- 宽度不足时依次隐藏在线人数、房间详情
- 最窄情况下保留 `chatbox · connected`

### Message Viewport

消息区继续使用现有 history entry 渲染模型，但调整视觉层级。

普通消息：

- 首条消息显示时间、昵称、正文
- 同一发送人连续消息弱化重复昵称
- 连续消息不改变消息身份，只是渲染时降低重复信息
- 时间戳保留，但颜色更弱

日期分割：

```text
──────── 2026-05-14 ────────
```

规则：

- 渲染时按消息时间插入，不写入 transcript
- 同一天只显示一次
- transcript replay 和实时消息使用同一规则

系统消息：

```text
        bob joined
```

规则：

- 系统消息使用 muted 样式
- 不和普通消息抢视觉权重
- 错误类系统消息仍保持明显，但不扩大成大块提示

### Reply Cards

现有回复卡片能力继续保留，并与新消息样式统一。

目标效果：

```text
[15:04] bob
  │ alice · 11:22
  │ hello world...
收到，我晚点处理
```

规则：

- 继续只做 TUI 渲染增强
- 不引入结构化 reply id
- 已撤回消息优先显示撤回占位，不泄露引用内容
- 窄终端下卡片内容按现有文本换行策略处理

### Attachment Cards

附件消息从“纯文本摘要”升级为轻卡片样式。

目标效果：

```text
[15:04] alice
  [image] cat.gif · 2.4 MB
  #att_abc123 · O open · D download
```

规则：

- 文件名、大小、类型、附件 id 保留
- TUI 中提示键盘操作；scrollback 不跟随变更
- 普通文件、图片、未知类型都使用同一轻卡片结构
- 附件上传/下载进度继续走现有状态栏，不塞进消息正文

### Input Area

底部输入区分成最多三层：

```text
reply preview
status notice
input line
```

规则：

- 没有回复草稿时隐藏 reply preview
- 没有状态提示时隐藏 status notice
- 输入行始终在最底部
- copy/revoke 模式的 action bar 保留，继续占用独立一行
- 窄窗口下优先保留输入行和 action bar，状态提示可截断

## Data Flow

不新增协议数据流。

渲染数据来自现有模型：

- history entries：普通消息、系统消息、附件消息、撤回状态
- roster/status：顶部在线人数和连接状态
- reply draft：底部回复预览
- status notice：底部状态提示
- attachment metadata：附件卡片内容

日期分割、连续发送人弱化、顶部栏字段裁剪都在 TUI 渲染层计算，不落盘。

## Components

建议在 `internal/tui/model.go` 内按现有模式拆小 helper，而不是引入新的 UI 框架。

建议边界：

- `renderTopBar`：顶部栏
- `renderTUIEntry` 或相邻 helper：普通消息、系统消息、回复卡片、附件卡片
- `renderDateSeparator`：日期分割
- `renderInputArea` 或现有底部渲染逻辑：回复预览、状态提示、输入框、action bar
- `truncateStatusParts`：窄终端字段裁剪

如果现有函数已经有对应职责，优先扩展现有函数，不为了命名强行重构。

## Error Handling

- roster 或房间名不可得：顶部栏隐藏对应字段，不显示 `unknown` 噪音
- 终端宽度过小：优先保证输入行可见，其次保留消息正文
- 附件元数据解析失败：回退现有附件摘要文本
- 日期时间异常：不插入日期分割，消息照常显示
- 样式能力不可用：按纯文本退化，不影响操作

## Compatibility

- scrollback 模式不变
- 现有快捷键不变
- 现有鼠标复制、引用、撤回、打开附件不变
- transcript 内容不变
- 历史同步内容不变
- 老版本消息照常显示

## Testing

需要覆盖：

1. 顶部栏显示连接状态、房间信息、在线人数
2. 窄窗口下顶部栏按优先级裁剪
3. 不同日期消息之间插入日期分割
4. 同一天多条消息只插入一次日期分割
5. 系统消息使用弱化布局
6. 连续同发送人消息不重复渲染完整头部
7. 回复卡片仍正确显示发送人、时间、摘要和正文
8. 附件消息渲染为轻卡片，保留附件 id
9. 回复预览、状态提示、输入行的高度计算正确
10. copy/revoke action bar 不被输入区改动破坏
11. 中文昵称和中文消息在常见宽度下不重叠

验证命令：

```bash
go test ./internal/tui -count=1
go test ./... -count=1
```

## Acceptance Criteria

- `--ui tui` 首屏能明确看到房间/连接/在线状态
- 普通消息、系统消息、回复、附件有清晰层级
- 小窗口不乱版，输入行始终可用
- 中文内容不明显错位或重叠
- 原有 TUI 交互不退化
- 全量测试通过

## Future Work

后续如果继续投入 TUI，可以再考虑：

- 搜索弹层
- 成员弹层
- 最近附件弹层
- 主题配置
- 更明确的未读分割线
- 点击回复卡片跳转原消息

这些不进入第一版，避免把视觉升级扩大成复杂聊天客户端。
