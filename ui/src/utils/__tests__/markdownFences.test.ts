import { describe, expect, it } from 'vitest';

import { getStreamingMarkdownSource, repairMalformedOuterFence } from '@/utils/markdownFences';

const body = [
  '继续完成 DocumentCore / GenOffice 阶段4。',
  '',
  '工作区：',
  '- DocumentCore：',
  '  D:/codes/2026/OnlyOfficeX2T/DocumentCore',
  '',
  '推荐工作顺序：',
  '',
  '```text',
  '读取相关代码和文档',
  '→ 一次性完成生产代码修改',
  '→ GPG 签名 Conventional Commit',
  '```',
  '',
  '验证策略：',
  '- 文档或 JSON 改动：只做 diff-check 和 JSON 解析。',
  '',
  '最终交付要求：',
  '- 保留用户原有未提交和 untracked 内容；',
];
const malformed = ['```text', ...body, '```'].join('\n');
const repaired = ['````text', ...body, '````'].join('\n');

describe('repairMalformedOuterFence', () => {
  it('keeps the workflow and following sections inside one outer code block', () => {
    expect(repairMalformedOuterFence(malformed)).toBe(repaired);
    expect(repairMalformedOuterFence(repaired)).toBe(repaired);
  });

  it('preserves CRLF, leading whitespace lines and the exact final newline', () => {
    const source = '\r\n' + malformed.replaceAll('\n', '\r\n') + '\r\n  \r\n';
    const expected = '\r\n' + repaired.replaceAll('\n', '\r\n') + '\r\n  \r\n';
    expect(repairMalformedOuterFence(source)).toBe(expected);
  });

  it.each(['md', 'markdown', 'plaintext', 'TEXT'])('accepts the %s wrapper label', label => {
    const source = ['```' + label, ...body, '```'].join('\n');
    const expected = ['````' + label, ...body, '````'].join('\n');
    expect(repairMalformedOuterFence(source)).toBe(expected);
  });

  it('supports tilde fences without changing the inner delimiters', () => {
    expect(repairMalformedOuterFence(malformed.replaceAll('```', '~~~'))).toBe(
      repaired.replaceAll('`', '~')
    );
  });

  it('lengthens an already longer conflicting outer fence', () => {
    const source = malformed.replaceAll('```', '````');
    const expected = [
      '`````text',
      ...body.map(line => line.replaceAll('```', '````')),
      '`````',
    ].join('\n');
    expect(repairMalformedOuterFence(source)).toBe(expected);
  });

  it.each([
    ['plain markdown', body.join('\n')],
    ['properly nested Markdown example', repaired],
    ['unclosed outer wrapper', ['```text', ...body].join('\n')],
    ['non-text outer language', ['```rust', ...body, '```'].join('\n')],
    ['unlabelled inner fence', malformed.replace('```text\n读取', '```\n读取')],
    ['message with an introduction', 'Introduction\n\n' + malformed],
    ['message with an epilogue', malformed + '\n\nEpilogue'],
    ['ordinary separate blocks', '```text\nfirst\n```\n\nprose\n\n```js\nsecond\n```'],
    ['intentional empty final block', '```text\nfirst\n```\n\nprose\n\n```\n```'],
    ['ambiguous extra block', malformed + '\n```js\nmore\n```'],
  ])('leaves %s unchanged', (_name, source) => {
    expect(repairMalformedOuterFence(source)).toBe(source);
  });
});

describe('getStreamingMarkdownSource', () => {
  it.each([
    '`',
    '``',
    '```',
    '```typescript',
    '````  ',
    '~',
    '~~',
    '~~~text',
    '  ```',
    '> ```text',
    '> > ~~~',
    '- ```js',
    '1. ```text',
    '    ```',
    '```\r',
  ])('holds an unfinished possible fence line: %j', tail => {
    expect(getStreamingMarkdownSource('Before\n\n' + tail)).toBe('Before\n\n');
    expect(getStreamingMarkdownSource('Before\n\n' + tail + '\n')).toBe('Before\n\n' + tail + '\n');
  });

  it.each(['Hello', 'Partial **bold', '`inline code`', '~~strike~~', 'const marker = ```'])(
    'keeps ordinary unfinished content visible: %j',
    tail => {
      expect(getStreamingMarkdownSource('Before\n' + tail)).toBe('Before\n' + tail);
    }
  );

  it('holds a fence even when it is the first delta', () => {
    expect(getStreamingMarkdownSource('```text')).toBe('');
  });
});
