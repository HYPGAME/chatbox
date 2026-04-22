# TUI Message Copy Design

**Problem**

`chatbox` 的 TUI 目前只能浏览消息，不能把某条消息稳定地复制到系统剪贴板。终端自身的鼠标选择在 alt-screen 和现有 viewport 交互下不可靠，因此需要一个明确的键盘复制能力。

**Goal**

在 `--ui tui` 模式下，允许用户用键盘选中一条聊天消息，并通过快捷键把该消息当前屏幕表现文本复制到系统剪贴板。

## Interaction

第一版采用“行级复制”方案。

- 只在 `--ui tui` 下生效
- 增加一个“当前选中消息”状态，只选中聊天消息，不选中 system/error 行
- `Up/Down` 在可复制消息之间移动选中项
- `Left/Right` 继续只作用于输入框光标
- `PgUp/PgDn/Home/End` 保持当前视口滚动语义
- `Ctrl+Y` 复制当前选中消息到系统剪贴板
- 复制成功后，底部状态区显示轻量提示，例如 `copied message`
- 复制失败或没有可复制消息时，只更新状态栏提示，不新增 system message

## Copy Semantics

复制内容按 TUI 当前渲染后的消息文本计算，而不是原始消息体。

- 单行消息复制完整渲染行
- 多行消息复制完整渲染文本
- 时间戳、发送人、正文、撤回后的当前可见文本都按屏幕表现复制
- system/error 行不可选中，因此不会被复制

这样做的好处是行为直观，用户复制到外部后看到的内容与 TUI 中看到的一致。

## Selection Behavior

选中状态与新消息追加行为需要稳定可预期。

- 默认进入 TUI 时，选中最新一条可复制消息
- 如果用户没有手动离开底部，新消息到来时选中继续跟随最新消息
- 一旦用户通过 `Up/Down` 向上移动，视为“手动离底”，新消息不再抢占当前选中项
- 当用户重新回到底部，后续新消息恢复自动跟随
- 如果当前没有任何可复制消息，选中状态为空

## Platform Behavior

第一版只做明确可控的平台支持。

- macOS：通过 `pbcopy` 写入系统剪贴板
- 其他平台：返回明确错误，例如 `copy unsupported on this platform`
- 不做隐式降级到临时文件、OSC52 或终端私有协议

这样可以避免在未验证的平台上引入不一致行为。

## Architecture

实现拆成三块：

1. TUI model 内维护“可复制消息索引”与“当前选中消息”
2. 复制能力抽成一个小接口，例如 `clipboardWriter func(string) error`
3. TUI 渲染层根据选中状态做轻量高亮，并在按键处理里触发复制

边界要求：

- 复制逻辑不能散落在多个按键分支里
- 选中状态必须只依赖 `historyKindMessage`
- 视口滚动和消息选中要通过单一辅助函数保持一致，避免新消息、翻页、键盘导航各改各的

## Data Flow

`Ctrl+Y` 的处理链路如下：

1. TUI 判断当前是否存在选中消息
2. 如果存在，取该消息当前渲染文本
3. 调用 `clipboardWriter`
4. 根据结果更新状态栏提示
5. 刷新 viewport，但不插入任何新的聊天记录

`Up/Down` 的处理链路如下：

1. 找到上一个或下一个 `historyKindMessage`
2. 更新当前选中索引
3. 若选中项不在当前 viewport 可见范围内，则调整 viewport
4. 刷新视图

## Error Handling

- 没有消息可复制：状态栏提示 `no message to copy`
- 当前平台不支持：状态栏提示 `copy unsupported`
- 系统剪贴板写入失败：状态栏提示短错误，例如 `copy failed`
- 任一失败都不影响聊天会话、输入框状态或消息历史

## Testing

必须覆盖以下行为：

- `Ctrl+Y` 成功复制当前选中消息
- `Up/Down` 只在消息之间移动，跳过 system/error 行
- 多行消息复制的是完整渲染文本
- 复制失败时状态栏给出错误提示
- 没有可复制消息时不会崩溃，也不会插入 system line
- 新消息追加时：
  - 在底部跟随模式下，选中项跟随最新消息
  - 手动离底后，选中项保持不动

## Non-Goals

- 鼠标拖拽选区复制
- 复制多条消息
- 复制输入框内容
- 在 scrollback 模式中复用同一套复制交互
- 非 macOS 平台的系统剪贴板集成
