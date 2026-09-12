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
