# TUI Reply Preview Design

## Context

当前 TUI 的“引用回复”本质上还是文本拼接：

- 用户在 `copy mode` 里选中消息后，`quote` 会把引用内容直接插入输入框
- 插入格式是多行 `>` 文本块
- 发送后，聊天区看到的也只是普通正文，不存在真正的“回复关系”

这套方案兼容性高，但有两个明显问题：

- 输入框一旦插入 `>` 块，继续编辑正文会变得笨重
- 发送后的展示较冗余，引用内容和正文缺乏层次，阅读成本高

当前底层消息仍然只有 `ID / From / Body / At`，不包含结构化 `reply_to` 字段，因此第一步优化应继续保持协议兼容。

## Goal

在不修改消息协议、不影响老版本客户端的前提下，把“引用回复”从“输入框里的大段文本”优化成更轻量的本地交互。

目标：

- 让输入区编辑回复正文更顺手
- 让发送后的引用展示更紧凑、更易读
- 保持所有消息仍然能以普通文本形式被旧客户端接收和显示
- 不破坏现有 transcript、history sync、revoke、attachment 流程

## Non-Goals

- 本阶段不引入结构化 `reply_to_message_id`
- 不支持点击引用跳转到原消息
- 不实现线程式对话
- 不要求 host 或 join 先升级后才能继续聊天
- 不修改现有 copy mode / revoke mode 的基本进入方式

## Recommended Approach

采用“两层表示”的轻量方案：

1. 本地编辑态使用独立的 `reply draft` 状态，不把引用原文直接塞进输入框
2. 真正发送时，再把 reply draft 格式化成兼容旧协议的普通文本消息
3. 本地渲染已发送消息时，识别这种文本格式，并尽量按“引用预览 + 正文”方式压缩展示

这样可以同时兼顾：

- 编辑体验
- 旧协议兼容
- 低改动成本

相比直接升级协议，这个方案不需要改 `session.Message`、房间同步协议和 transcript 数据结构，适合先落地。

## Interaction Design

### Reply Draft

当用户在 `copy mode` 里执行 `quote`：

- 不再把 `>` 文本块直接写进输入框
- 改为设置一个本地 `reply draft`
- 输入框上方显示一行淡色 reply bar

建议样式：

```text
reply aaa [15:04] hello world...   [x]
```

规则：

- 只显示一行摘要
- 摘要优先取原消息首行，并截断到固定长度
- `Esc` 可取消 reply draft
- 鼠标点击 `[x]` 也可取消
- 输入框只保留用户真正要发送的正文

### Send Behavior

发送时：

- 如果没有 `reply draft`，维持现有发送逻辑
- 如果存在 `reply draft`，则把 reply draft 和输入框正文组合成普通文本消息再发送

推荐发送文本格式：

```text
> aaa [15:04] hello world...
actual reply body
```

约束：

- 引用只保留单行摘要，不再发送完整多行引用块
- 引用行与正文之间只保留一个换行
- 如果正文为空，则不允许发送，仅提示用户继续输入
- 发送成功或失败后都清除 reply draft，避免重复引用

## Rendered Message Design

聊天区对“引用格式消息”做本地识别和紧凑渲染。

显示目标：

- 第一行以更淡的样式显示引用摘要
- 第二行显示真实正文
- 不额外插入空白行

建议效果：

```text
[15:04] bob: reply aaa [14:59] hello world...
            收到，我晚点处理
```

渲染规则：

- 仅识别“单行引用摘要 + 正文”的新格式
- 老的多行 `>` 文本块继续按原样显示，不做复杂兼容改写
- 如果正文为空，整条消息按普通文本显示，避免误判
- 多行正文仍按原本换行渲染，但首屏只强调第一行引用摘要

## Attachment And Revoke Handling

reply draft 的摘要生成需要兼容特殊消息：

- 如果原消息是图片或文件，摘要显示附件语义，例如：
  - `[图片] cat.gif`
  - `[文件] demo.zip`
- 如果原消息已撤回，摘要显示：
  - `已撤回一条消息`
- 如果原消息正文为空或无法提取，使用保守占位：
  - `消息`

这保证 reply draft 不会把附件协议正文或过长内容原样塞进 UI。

## State Model

为 TUI model 增加本地 reply draft 状态即可，不动网络协议。

建议字段：

- 目标消息的 `messageID`
- 发送人
- 发送时间
- 摘要文本

用途：

- 输入区 reply bar 渲染
- 发送时生成兼容文本
- 取消时清空状态

不需要把 reply draft 写入 transcript。transcript 里只保存最终发送出的普通文本消息。

## Error Handling

- 没有选中消息时执行 `quote`：沿用当前错误提示
- reply draft 存在但正文为空时按发送：提示 `reply body required`
- reply 目标已不存在：允许继续发送，因为发送使用的是已缓存摘要
- 撤回消息仍允许被引用，但摘要固定为 `已撤回一条消息`

## Compatibility

本设计对老版本兼容：

- 老版本客户端只会看到普通文本：
  - 第一行 `> aaa [15:04] hello world...`
  - 第二行及之后是正文
- 新版本客户端可把这类文本本地压缩成 reply preview
- 不修改 host 转发、历史同步、离线 transcript 存储格式

这意味着：

- 老客户端不会崩
- 新旧版本可以混聊
- 体验差异仅体现在新客户端的本地渲染

## Testing

需要覆盖以下测试：

1. `quote` 不再把 `>` 块直接插入输入框，而是创建 reply draft
2. reply draft 渲染为单行 reply bar
3. `Esc` 可以取消 reply draft
4. 发送时 reply draft 会被格式化为兼容文本消息
5. reply draft 存在但正文为空时不能发送
6. 已发送引用消息在 TUI 中渲染为“引用摘要 + 正文”，且无额外空行
7. 老格式多行 `>` 引用仍能按普通文本显示
8. 附件消息引用时，reply draft 摘要显示附件标签
9. 撤回消息引用时，reply draft 摘要显示撤回占位

## Future Work

如果后续要做真正的“回复某条消息”，再进入第二阶段：

- 消息协议新增 `reply_to_message_id`
- transcript 保存结构化 reply 元数据
- 渲染层支持点击引用跳转原消息
- 原消息撤回后，引用卡片联动更新

这一步成本明显更高，不应与当前轻量优化绑定在一起。
