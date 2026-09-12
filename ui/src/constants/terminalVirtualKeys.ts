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
