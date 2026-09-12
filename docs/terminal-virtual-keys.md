# 终端虚拟按键条 与 快捷指令默认项调整

## 影响文件清单

| 文件 |
| --- |
| `ui/src/stores/settings.ts` |
| `ui/src/components/terminal/TerminalPanel.vue` |
| `ui/src/components/terminal/TerminalPanel.vue` |
| `ui/src/constants/terminalVirtualKeys.ts` |
| `ui/src/constants/__tests__/terminalVirtualKeys.test.ts` |
| `ui/src/i18n/locales/zh-CN.ts` |
| `ui/src/i18n/locales/en-US.ts` |

---

# 改动一：精简终端快捷指令默认项

## 目的

默认快捷指令原本是三项：`Claude Code`（`claude`）、`Claude Code Router`（`ccr code`）、`Codex`（`codex`）。其中 `ccr` 入口与 `claude` 高度重合，收敛为两项；同时让默认命令直接带上免确认参数，减少每次启动 agent 时的交互打断。

## `ui/src/stores/settings.ts`

定位 `export const DEFAULT_TERMINAL_QUICK_ACTIONS`（原第 356 行附近，紧跟在 `DEFAULT_DAILY_TIP_SETTINGS` 之后）。

改动前：

```ts
export const DEFAULT_TERMINAL_QUICK_ACTIONS: TerminalQuickAction[] = [
  {
    id: 'claude',
    name: 'Claude Code',
    command: 'claude',
    icon: 'claude',
    enabled: true,
    stacked: false,
  },
  {
    id: 'ccr',
    name: 'Claude Code Router',
    command: 'ccr code',
    icon: 'claude',
    enabled: true,
    stacked: false,
  },
  {
    id: 'codex',
    name: 'Codex',
    command: 'codex',
    icon: 'codex',
    enabled: true,
    stacked: false,
  },
];
```

改动后：

```ts
export const DEFAULT_TERMINAL_QUICK_ACTIONS: TerminalQuickAction[] = [
  {
    id: 'claude',
    name: 'Claude',
    command: 'claude --dangerously-skip-permissions',
    icon: 'claude',
    enabled: true,
    stacked: false,
  },
  {
    id: 'codex',
    name: 'Codex',
    command: 'codex --yolo',
    icon: 'codex',
    enabled: true,
    stacked: false,
  },
];
```

净效果：删除整个 `ccr` 对象，`claude` 的 `name` 与 `command` 改写，`codex` 的 `command` 改写。`icon` / `enabled` / `stacked` 均不变。

## `ui/src/components/terminal/TerminalPanel.vue`

定位 `resolveAgentCommand`（原第 1888 行附近，紧跟 `resolveQuickActionCommand` 之后）。去掉对 `ccr` 的兜底查找，避免指向已删除的默认项。

```diff
 function resolveAgentCommand(agent: 'claude' | 'codex') {
   if (agent === 'claude') {
-    return resolveQuickActionCommand('claude') || resolveQuickActionCommand('ccr') || 'claude';
+    return resolveQuickActionCommand('claude') || 'claude';
   }
   return resolveQuickActionCommand('codex') || 'codex';
 }
```

`resolveAgentCommand('claude')` 的返回值会传给 `AISessionHistoryDialog` 的 `claude-command` prop，所以这里必须与默认项同步，否则历史对话框会拿到一个用户配置里不存在的命令。

## 兼容性与风险

- `sanitizeTerminalQuickActions`（`settings.ts` 约 1718 行）会遍历 `DEFAULT_TERMINAL_QUICK_ACTIONS` 把缺失的默认项补回用户已有配置。因此老用户本地保留的 `ccr` 项不会被删除，但升级后不会再自动补齐；新装用户看不到该入口。
- 老用户若已保存过 `claude` / `codex` 项，其 `command` 不会被覆盖，仍是不带参数的旧命令。只有新装用户或点击「重置快捷指令」（`handleResetTerminalQuickActions`，`GeneralSettings.vue` 约 4049 行）的用户才会拿到带参数的版本。
- **需要注意的行为变化**：`--dangerously-skip-permissions` 与 `--yolo` 会跳过 agent 的权限确认。这是有意为之的默认值，若需要严格确认，在设置里把命令改回不带参数的形式。

---

# 改动二：新增终端虚拟按键条

## 目的

终端面板底部增加一条可开关的虚拟按键条，点击后直接向当前终端会话发送对应控制序列。解决触屏设备没有物理按键、以及 `Esc` / `Ctrl+C` 这类键在移动端不便输入的问题。默认关闭。

## 新增 `ui/src/constants/terminalVirtualKeys.ts`

按键定义与查询工具，纯数据 + 纯函数，不依赖 Vue，便于单测。

```ts
/**
 * 终端虚拟按键：点击后往当前终端发送对应的控制序列。
 * 主要解决触屏设备没有物理按键，以及部分组合键不便直接输入的场景。
 */
export interface TerminalVirtualKey {
  id: string;
  label: string;
  data: string;
}

export interface TerminalVirtualKeyGroup {
  id: string;
  keys: TerminalVirtualKey[];
}

export const TERMINAL_VIRTUAL_KEY_GROUPS: readonly TerminalVirtualKeyGroup[] = [
  {
    id: 'symbol',
    keys: [
      { id: 'slash', label: '/', data: '/' },
      { id: 'at', label: '@', data: '@' },
    ],
  },
  {
    id: 'control',
    keys: [
      { id: 'esc', label: 'Esc', data: '\x1b' },
      { id: 'tab', label: 'Tab', data: '\t' },
      { id: 'shift-tab', label: 'Shift+Tab', data: '\x1b[Z' },
      { id: 'ctrl-c', label: 'Ctrl+C', data: '\x03' },
      { id: 'ctrl-d', label: 'Ctrl+D', data: '\x04' },
      { id: 'ctrl-z', label: 'Ctrl+Z', data: '\x1a' },
      { id: 'enter', label: 'Enter', data: '\r' },
    ],
  },
  {
    id: 'nav',
    keys: [
      { id: 'arrow-up', label: '↑', data: '\x1b[A' },
      { id: 'arrow-down', label: '↓', data: '\x1b[B' },
      { id: 'arrow-left', label: '←', data: '\x1b[D' },
      { id: 'arrow-right', label: '→', data: '\x1b[C' },
      { id: 'home', label: 'Home', data: '\x1b[H' },
      { id: 'page-up', label: 'PgUp', data: '\x1b[5~' },
      { id: 'page-down', label: 'PgDn', data: '\x1b[6~' },
      { id: 'end', label: 'End', data: '\x1b[F' },
      { id: 'delete', label: 'Del', data: '\x1b[3~' },
    ],
  },
];

export const TERMINAL_VIRTUAL_KEYS: readonly TerminalVirtualKey[] =
  TERMINAL_VIRTUAL_KEY_GROUPS.flatMap(group => group.keys);

export function findTerminalVirtualKey(id: string): TerminalVirtualKey | undefined {
  return TERMINAL_VIRTUAL_KEYS.find(key => key.id === id);
}

export function resolveTerminalVirtualKeyData(id: string): string {
  return findTerminalVirtualKey(id)?.data ?? '';
}
```

设计要点：

- 分三组（`symbol` / `control` / `nav`），模板按组渲染，组间用更大的 gap 做视觉分隔。
- `data` 直接写 pty 期望的字节，分三类：
  - **普通字符**：`/`、`@` 直接写字面量。见下方「为什么要有 symbol 组」。
  - **控制字符**：`\x1b`（Esc）、`\t`（Tab）、`\x03`（Ctrl+C）、`\x04`（Ctrl+D）、`\x1a`（Ctrl+Z）、`\r`（Enter）。
  - **CSI 序列**：`\x1b[Z`（Shift+Tab，back-tab）、`\x1b[A/B/C/D`（方向键）、`\x1b[H`（Home）、`\x1b[F`（End）、`\x1b[5~`（PgUp）、`\x1b[6~`（PgDn）、`\x1b[3~`（Delete）。
- 几个容易写错的取值：
  - 方向键 `\x1b[C` 是**右**、`\x1b[D` 是**左**，别按字母顺序想当然。
  - Enter 用 `\r`（CR，0x0D）不是 `\n`。pty 在 icanon 模式下期望的行结束符是 CR，发 `\n` 部分程序不认。
  - Shift+Tab 没有对应的控制字符，pty 收不到修饰键状态，只能靠 `\x1b[Z` 这个 back-tab 序列表达。
  - Delete 用 `\x1b[3~`（向后删除，删光标右侧字符），不是 `\x7f`——后者是 Backspace（向前删除）。两者是不同的键，别混。
- `resolveTerminalVirtualKeyData` 对未知 id 返回空字符串而不是抛错，调用方不需要 try/catch。
- 数组标成 `readonly`，避免被就地改写。

### 为什么要有 symbol 组

`/` 和 `@` 是普通可打印字符，物理键盘上直接敲就行，放进虚拟键条是为了绕开移动端软键盘的一个坑。

xterm.js 主要靠 `keydown` 的 `event.key` 映射输入，而移动端软键盘按下**符号层**的键时，`keydown` 往往只给出 `key: 'Unidentified'` / `keyCode: 229`，真正的字符只从 `input` / `compositionupdate` 事件出来。xterm 5.x 的 CompositionHelper 能兜一部分，但符号层的单字符输入在 Gboard、iOS 中文键盘等输入法上不会触发 `compositionend`，于是既没走成 keydown 路径、也没被 composition 收尾 flush，字符就丢了。表现就是字母数字正常、偏偏符号打不出来。

虚拟键走 `send(sessionId, { type: 'input', data })` 直接推给 pty，完全不经过浏览器键盘事件，所以不受输入法影响。

选这两个字符是因为它们在 Claude Code 里分别是 slash 命令和 @ 文件引用的触发字符，移动端用不了等于半个功能不可用。同理 Enter 也放进来了——软键盘的回车在部分输入法下同样走不通 keydown 路径。

注意这个成因是从代码路径和 xterm 输入机制推断的，未经真机事件级验证。要确认的话在手机上开远程调试，给 `terminal.textarea` 挂监听打印 `keydown` 的 `key`/`keyCode` 与 `input`/`compositionupdate` 的值，敲一下 `/` 就能看出是哪一环断的。

## `ui/src/components/terminal/TerminalPanel.vue`

共 6 处插入，全部是新增，没有删除已有代码。

### 1. 模板：按键条

插在 `.panel-body` 里终端列表 `v-for` 之后、包裹 div 的闭合标签之前（原第 409 行附近，`<TerminalTabPane ... :is-mobile="isMobile" />` 之后）：

```vue
    <div v-if="shouldShowVirtualKeys" class="virtual-key-bar">
      <div v-for="group in virtualKeyGroups" :key="group.id" class="virtual-key-group">
        <button
          v-for="key in group.keys"
          :key="key.id"
          type="button"
          class="virtual-key"
          :title="t('terminal.virtualKeySend', { key: key.label })"
          @click="handleVirtualKeyPress(key)"
        >
          {{ key.label }}
        </button>
      </div>
    </div>
```

用原生 `<button type="button">` 而不是 `NButton`：数量多（18 个），原生按钮更轻，且天然支持键盘聚焦。`type="button"` 必须写，避免在表单上下文里触发提交。

### 2. 导入

紧跟 `@/constants/terminalRenderMode` 的导入之后（原第 502 行附近）：

```ts
import {
  TERMINAL_VIRTUAL_KEY_GROUPS,
  type TerminalVirtualKey,
} from '@/constants/terminalVirtualKeys';
```

### 3. 持久化状态

紧跟 `showBranchFilter` 之后（原第 640 行附近），与相邻开关保持同一写法：

```ts
const showVirtualKeys = useStorage('terminal-show-virtual-keys', false);
```

用 `useStorage`（VueUse，写 localStorage）而不是 `settingsStore`，与同区域的 `terminal-auto-resize`、`terminal-show-branch-filter` 一致：这是纯本地视图偏好，不需要同步到后端设置。默认 `false`，桌面端不改变现状。

### 4. 设置菜单项

在 `settingsMenuOptions` 里，`branch-filter-toggle` 之后、`default-open-in-mirror-mode` 之前（原第 893 行附近）：

```ts
  {
    label: t('terminal.showVirtualKeys'),
    key: 'virtual-keys-toggle',
    icon: showVirtualKeys.value
      ? () => h(NIcon, null, { default: () => h(CheckmarkOutline) })
      : undefined,
  },
```

勾选态用 `CheckmarkOutline` 图标表达，与同菜单其它开关项写法一致。

### 5. 显示条件与点击处理

紧跟 `handleEditorSelect` 之后、`enabledQuickActions` 之前（原第 1776 行附近）：

```ts
const virtualKeyGroups = TERMINAL_VIRTUAL_KEY_GROUPS;

// 只有激活的是真实终端时才显示虚拟键，空标签没有可发送的会话
const shouldShowVirtualKeys = computed(
  () => showVirtualKeys.value && Boolean(activeId.value) && !isEmptyTab(activeId.value)
);

function handleVirtualKeyPress(key: TerminalVirtualKey) {
  const sessionId = activeId.value;
  if (!sessionId || isEmptyTab(sessionId)) {
    return;
  }
  if (!send(sessionId, { type: 'input', data: key.data })) {
    message.warning(t('terminal.virtualKeyDisconnected'));
    return;
  }
  focusSessionInStore(sessionId);
}
```

依赖的既有符号（都已在该组件内可用，无需新增导入）：

- `activeId`：computed（约 1267 行），当前激活标签 id。
- `isEmptyTab(tabId)`：约 693 行，判断是否是 `EMPTY_TAB_PREFIX` 开头的空标签。
- `send`、`focusSession: focusSessionInStore`：从 `useTerminalClient(projectIdRef)` 解构（约 990 行）。`send` 返回 `boolean`，false 表示 socket 未连接。
- `message`：`useMessage()`（537 行）；`t`：`useLocale()`（541 行）。

三个关键点：

- `shouldShowVirtualKeys` 里排除空标签——空标签没有 pty，按键无处可发。
- `send` 失败要给用户反馈，否则点了没反应像是 bug。
- 发送成功后调 `focusSessionInStore(sessionId)` 把焦点交回终端。不做这一步，焦点会停在刚点的 `<button>` 上，用户接下来的物理键盘输入会打到按钮而不是终端。

### 6. 菜单选择分支

在 `handleSettingsMenuSelect` 里，`branch-filter-toggle` 分支之后、`default-open-in-mirror-mode` 之前（原第 2607 行附近）：

```ts
  } else if (key === 'virtual-keys-toggle') {
    showVirtualKeys.value = !showVirtualKeys.value;
    scheduleResizeAll();
```

`scheduleResizeAll()`（约 1618 行，`useDebounceFn` 包装）必须调：按键条占据垂直空间，显示/隐藏会改变 `.panel-body` 高度，xterm 需要重新计算行列数，否则内容会被截断或留空。

### 7. 样式

插在 `.panel-body` 规则之后、`.tab-label` 之前（原第 2959 行附近）：

```css
.virtual-key-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
  flex-shrink: 0;
  padding: 6px 12px;
  background-color: var(--app-surface, var(--n-card-color, #fff));
  border-top: 1px solid var(--app-border, var(--n-border-color));
}

.virtual-key-group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.virtual-key {
  min-width: 32px;
  height: 26px;
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  line-height: 1;
  color: var(--app-text-secondary, var(--n-text-color-2, #666));
  background: var(--app-surface-hover, var(--n-color-hover, rgba(0, 0, 0, 0.04)));
  border: 1px solid var(--app-border, var(--n-border-color, #e0e0e0));
  border-radius: 5px;
  cursor: pointer;
  transition:
    color 0.2s ease,
    background-color 0.2s ease;
}

.virtual-key:hover {
  color: var(--app-text-primary, var(--n-text-color, #1f1f1f));
  background: var(--app-surface-active, var(--n-color-pressed, rgba(0, 0, 0, 0.08)));
}

.virtual-key:active {
  color: var(--app-accent, var(--n-color-primary, #3b82f6));
  border-color: var(--app-accent, var(--n-color-primary, #3b82f6));
}

.virtual-key:focus-visible {
  outline: 2px solid var(--app-focus-ring, var(--n-color-primary, #3b82f6));
  outline-offset: 1px;
}
```

要点：

- 颜色一律走 `--app-*` 主题变量，并保留 naive-ui 变量（`--n-*`）作为第二级兜底、字面值作为第三级，保证暗色主题下不会出现硬编码白底。
- `.virtual-key-bar` 用 `gap: 4px 12px`（行间距 4、列间距 12）把两个组在视觉上分开；`flex-wrap: wrap` 让窄屏自动折行。
- `flex-shrink: 0` 防止按键条被终端区域挤扁。
- `font: inherit` 重置原生 button 的默认字体，再单独指定 `font-size`。
- `:focus-visible` 轮廓是可访问性要求，键盘 Tab 遍历时必须能看到当前焦点。

## 新增 `ui/src/constants/__tests__/terminalVirtualKeys.test.ts`

```ts
import { describe, expect, it } from 'vitest';

import {
  TERMINAL_VIRTUAL_KEYS,
  TERMINAL_VIRTUAL_KEY_GROUPS,
  findTerminalVirtualKey,
  resolveTerminalVirtualKeyData,
} from '@/constants/terminalVirtualKeys';

describe('terminalVirtualKeys', () => {
  it('keeps every key id unique so lookups stay unambiguous', () => {
    const ids = TERMINAL_VIRTUAL_KEYS.map(key => key.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('flattens groups without dropping keys', () => {
    const grouped = TERMINAL_VIRTUAL_KEY_GROUPS.reduce(
      (total, group) => total + group.keys.length,
      0
    );
    expect(TERMINAL_VIRTUAL_KEYS).toHaveLength(grouped);
  });

  it('sends the control characters a pty expects', () => {
    expect(resolveTerminalVirtualKeyData('esc')).toBe('\x1b');
    expect(resolveTerminalVirtualKeyData('enter')).toBe('\r');
    expect(resolveTerminalVirtualKeyData('tab')).toBe('\t');
    expect(resolveTerminalVirtualKeyData('ctrl-c')).toBe('\x03');
    expect(resolveTerminalVirtualKeyData('ctrl-z')).toBe('\x1a');
  });

  it('sends the back-tab sequence for shift+tab', () => {
    expect(resolveTerminalVirtualKeyData('shift-tab')).toBe('\x1b[Z');
  });

  // 移动端软键盘符号层的 / 和 @ 常常发不出 keydown，走 pty 直发绕开输入法
  it('sends plain symbols verbatim', () => {
    expect(resolveTerminalVirtualKeyData('slash')).toBe('/');
    expect(resolveTerminalVirtualKeyData('at')).toBe('@');
  });

  it('sends CSI sequences for navigation keys', () => {
    expect(resolveTerminalVirtualKeyData('arrow-up')).toBe('\x1b[A');
    expect(resolveTerminalVirtualKeyData('arrow-right')).toBe('\x1b[C');
    expect(resolveTerminalVirtualKeyData('page-down')).toBe('\x1b[6~');
    expect(resolveTerminalVirtualKeyData('delete')).toBe('\x1b[3~');
  });

  it('never yields empty data for a declared key', () => {
    for (const key of TERMINAL_VIRTUAL_KEYS) {
      expect(key.data).not.toBe('');
      expect(key.label.trim()).not.toBe('');
    }
  });

  it('resolves unknown ids to nothing instead of throwing', () => {
    expect(findTerminalVirtualKey('nope')).toBeUndefined();
    expect(resolveTerminalVirtualKeyData('nope')).toBe('');
  });
});
```

覆盖 8 项：id 唯一性、分组展开无遗漏、控制字符取值、back-tab 序列、字面量符号、CSI 序列取值、无空 `data`/`label`、未知 id 兜底。

唯一性、非空、分组展开这三项是遍历式断言，新增按键会自动被覆盖，不需要补测试。

## i18n 文案

两个文件都插在 `terminal.showBranchFilter` 之后、`// Process status` 注释之前，保持两份 locale 的 key 顺序一致。

`ui/src/i18n/locales/zh-CN.ts`（约 497 行）：

```ts
    showVirtualKeys: '显示虚拟按键条',
    virtualKeySend: '发送 {key}',
    virtualKeyDisconnected: '终端未连接，按键未发送',
```

`ui/src/i18n/locales/en-US.ts`（约 515 行）：

```ts
    showVirtualKeys: 'Show virtual key bar',
    virtualKeySend: 'Send {key}',
    virtualKeyDisconnected: 'Terminal is not connected, key not sent',
```

`virtualKeySend` 带 `{key}` 插值，模板里以 `t('terminal.virtualKeySend', { key: key.label })` 调用，渲染为按钮 `title`。

---

# 复现步骤

在 `25a1ae4` 或等价基线上按顺序执行：

1. 改 `ui/src/stores/settings.ts` 的 `DEFAULT_TERMINAL_QUICK_ACTIONS` + `TerminalPanel.vue` 的 `resolveAgentCommand` → 提交为 `refactor(terminal)`。
2. 新增 `terminalVirtualKeys.ts` 与其单测 → 先跑 `npx vitest run src/constants/__tests__/terminalVirtualKeys.test.ts`，确认常量层独立可用。
3. 加中英文文案（两份 locale 同步，key 顺序一致）。
4. 改 `TerminalPanel.vue` 的 6 处插入，顺序建议：导入 → 状态 → 逻辑 → 菜单项 → 菜单分支 → 模板 → 样式。先补逻辑再写模板，避免中间态类型报错。
5. 验证后提交为 `feat(terminal)`。

## 验证命令

在 `ui/` 目录下执行：

```bash
npx vitest run src/constants/__tests__/terminalVirtualKeys.test.ts
npx vue-tsc --build
npx oxlint --config .oxlintrc.json <改动文件...> --deny-warnings
npx prettier --check <改动文件...>
```

实际结果：单测 8 项通过，`vue-tsc --build` 无输出，oxlint 0 warning / 0 error，prettier 全部符合。全量检查是 `npm run check`（并行跑 type-check + lint + test）。

## 手动验收点

- 设置菜单出现「显示虚拟按键条」，勾选态图标随开关变化；刷新页面后开关状态保持（localStorage `terminal-show-virtual-keys`）。
- 开关切换后终端内容不被截断也不留空白（`scheduleResizeAll` 生效）。
- 切到空标签时按键条消失，切回真实终端恢复。
- 在 `vim` 或交互式 TUI 里点方向键、`Esc`、`PgUp`/`PgDn`，行为与物理键一致；左右方向不能反（`\x1b[C` 右、`\x1b[D` 左）。
- **移动端真机**：点 `/` 和 `@` 能正常输入（这是 symbol 组存在的理由，桌面端测不出问题）；点 `Enter` 能提交当前行。
- 点 `Shift+Tab`，在 Claude Code 里应切换 auto-accept 模式。**这条未经真机验证**：多数 TUI 库（含 ink）认 `\x1b[Z`，但若无反应说明目标程序监听的是别的形式，需要另找序列。
- 点完按键后直接敲物理键盘，输入应进入终端而不是留在按钮上。
- 断开的终端上点按键，弹出「终端未连接，按键未发送」提示。
- 暗色主题下按键条背景/边框跟随主题，不出现白底。
- 键盘 Tab 遍历到按键时有可见轮廓。

## 后续扩展

新增按键只需在 `TERMINAL_VIRTUAL_KEY_GROUPS` 里加条目，`TERMINAL_VIRTUAL_KEYS`、查询函数、模板渲染和单测里的唯一性/非空校验都会自动覆盖，无需改 `TerminalPanel.vue`。加新分组同理，模板按组循环渲染。
