# TUI Copy Mode And Quote Reply Design

**Problem**

当前 `chatbox` 的 TUI 已支持键盘选中消息并复制，但交互仍然不够清晰：

- 用户看不出自己是否处于“复制/选择”状态
- 没有显式退出复制相关交互的方式
- 不能基于当前选中消息快速做“引用回复”

这会让 `Up/Down`、`Enter`、复制和正常输入之间的边界变得模糊，影响日常聊天体验。

**Goal**

在 `--ui tui` 模式下引入一个显式的 `copy mode`，让消息复制和引用回复都在这个模式内完成，同时保持现有协议兼容，不要求其他客户端升级。

## Interaction

第一版采用“显式复制模式 + 文本引用回复”方案。

- 只在 `--ui tui` 下生效
- `Ctrl+Y` 从普通输入态进入 `copy mode`
- 进入 `copy mode` 后，`Up/Down` 在聊天消息之间移动选中项
- `Ctrl+Y` 在 `copy mode` 内执行“复制当前选中消息”
- `Enter` 在 `copy mode` 内把当前选中消息作为引用文本插入输入框
- `Esc` 退出 `copy mode`
- 非 `copy mode` 下，`Up/Down` 不再承担消息选择职责，保持输入框原有行为
- `Left/Right` 始终只作用于输入框光标
- `PgUp/PgDn/Home/End` 保持当前视口滚动语义

这样可以把“编辑输入”和“操作历史消息”拆成两个明确状态，减少误触发。

## Copy Mode Behavior

`copy mode` 是一个轻量、显式、可退出的模式。

- 默认不激活
- 进入后，如果存在可选消息，默认选中最新一条聊天消息
- 只允许选中 `historyKindMessage`
- `system/error` 行不可选中
- 新消息到来时：
  - 如果当前选中仍处于底部跟随状态，则选中继续跟随最新消息
  - 如果用户已向上移动过选中项，则保持当前选中，不抢焦点
- 退出 `copy mode` 后，不再显示复制相关高亮
- 退出 `copy mode` 不清空输入框内容

## Quote Reply Format

引用回复采用“纯文本插入输入框”方案，不改聊天协议。

插入格式如下：

```text
> alice [11:00]
> hello world

```

具体规则：

- 第一行固定为 `> <sender> [HH:MM]`
- 后续每一行正文都加 `> ` 前缀
- 多行消息按原始正文逐行展开
- 引用块后额外补一个空行，光标落在空行后，用户直接继续输入回复正文
- 如果输入框已有内容，则在现有内容后补足一个换行，再插入引用块，避免粘连
- 被撤回消息不能作为“原文回显”引用，插入的正文应为当前可见文本，即 `已撤回一条消息`

这样可以保证：

- 老版本客户端无需理解新协议
- Android、macOS、路由器 host、旧 join 都能看到一致的引用文本
- 本地离线历史和转录文件也保持普通文本语义

## Mode Relationship

`copy mode` 与现有 `revoke mode` 必须互斥。

- 如果用户在 `revoke mode` 中按 `Ctrl+Y`，应先退出 `revoke mode`，再进入 `copy mode`
- 如果用户在 `copy mode` 中按 `Ctrl+R`，应先退出 `copy mode`，再进入 `revoke mode`
- 两个模式都使用状态栏和输入区 hint 明确提示当前模式和可用按键
- 任一模式下都不应把模式提示写入聊天历史

## Rendering

TUI 需要把当前状态表达得更明确。

- `copy mode` 下，当前选中消息保持现有高亮形式
- 普通模式下，不显示复制选中高亮
- 状态栏优先展示模式态提示或复制结果提示，例如：
  - `copy mode`
  - `copied message`
  - `copy unsupported`
- 输入框 hint 在 `copy mode` 下切换为：
  - `copy mode: Up/Down select / Enter quote / Ctrl+Y copy / Esc cancel`

## Data Flow

进入 `copy mode`：

1. 校验当前 UI 模式为 `tui`
2. 如无可选消息，保留普通模式并展示 `no message to copy`
3. 如有可选消息，设置 `copy mode = true`
4. 默认选中最新消息并刷新 viewport

`Ctrl+Y` 在 `copy mode` 内复制：

1. 读取当前选中消息的渲染文本
2. 调用 `clipboardWriter`
3. 更新状态栏提示
4. 保持 `copy mode`，不退出

`Enter` 在 `copy mode` 内引用回复：

1. 读取当前选中消息
2. 生成引用文本块
3. 追加写入输入框
4. 退出 `copy mode`
5. 保持焦点在输入框，不自动发送

`Esc` 在 `copy mode` 内退出：

1. 清除 `copy mode` 标记
2. 清除复制高亮
3. 刷新 viewport

## Error Handling

- 没有可选消息时按 `Ctrl+Y`：状态栏提示 `no message to copy`
- 平台不支持系统剪贴板：状态栏提示 `copy unsupported`
- 剪贴板写入失败：状态栏提示 `copy failed`
- `Enter` 引用回复失败时不发送消息，也不清空输入框
- 任一失败都不新增 system message，不影响连接状态，不破坏当前聊天记录

## Testing

必须覆盖以下行为：

- `Ctrl+Y` 从普通模式进入 `copy mode`
- `Esc` 从 `copy mode` 退出并移除选中高亮
- `Ctrl+Y` 在 `copy mode` 内成功复制当前选中消息
- 无消息时按 `Ctrl+Y` 不进入 `copy mode`，只显示错误提示
- `Enter` 在 `copy mode` 内把引用文本插入输入框
- 多行消息引用时，每一行都带 `> ` 前缀
- 输入框已有内容时，引用文本正确追加，不与原内容粘连
- `copy mode` 与 `revoke mode` 互斥切换正确
- 新消息追加时：
  - 在底部跟随状态下，`copy mode` 选中跟随最新消息
  - 手动离底后，选中保持不动

## Non-Goals

- 协议级消息引用
- 在远端客户端显示“回复了某条消息”的结构化卡片
- 多选复制或批量引用
- 鼠标选区复制
- `scrollback` 模式下复用同一套交互
