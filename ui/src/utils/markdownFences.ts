import { Marked } from 'marked';

const fenceLexer = new Marked({ gfm: true });

/**
 * Opt-in display repair for a completed message wrapped around one same-length
 * labelled fence. Normal multiple blocks, unclosed wrappers and ambiguous cases
 * stay verbatim; this is deliberately not a general Markdown repair pass.
 */
export function repairMalformedOuterFence(source: string): string {
  if (!source.includes('```') && !source.includes('~~~')) {
    return source;
  }

  const lines = source.split('\n');
  const first = lines.findIndex(line => line.trim() !== '');
  let last = lines.length - 1;
  while (last > first && lines[last].trim() === '') {
    last -= 1;
  }
  if (first < 0 || first === last) {
    return source;
  }

  const opening = /^(`{3,}|~{3,})[ \t]*(text|plaintext|md|markdown)[ \t]*\r?$/i.exec(lines[first]);
  if (!opening) {
    return source;
  }

  const fences: Array<{ line: number; marker: string; info: string }> = [];
  for (let index = first; index <= last; index += 1) {
    const match = /^ {0,3}(`{3,}|~{3,})([^\r\n]*)\r?$/.exec(lines[index]);
    if (match) {
      fences.push({ line: index, marker: match[1], info: match[2].trim() });
    }
  }

  // Four lines: outer opener, labelled inner opener, premature closer, and an
  // orphaned outer closer after more prose. Leave other fence layouts alone.
  if (fences.length !== 4) {
    return source;
  }
  const [outer, inner, innerClose, outerClose] = fences;
  if (
    outer.line !== first ||
    outerClose.line !== last ||
    fences.some(fence => fence.marker !== opening[1]) ||
    !/^[a-z0-9][a-z0-9_+.#-]*$/i.test(inner.info) ||
    innerClose.info !== '' ||
    outerClose.info !== '' ||
    lines
      .slice(innerClose.line + 1, last)
      .join('\n')
      .trim() === ''
  ) {
    return source;
  }

  // Confirm the parser's actual failure shape, including an empty final code
  // token. Surrounding blank lines and a detection-only newline must not turn
  // that orphaned closer into a whitespace-filled block. Output stays verbatim.
  const envelope = lines.slice(first, last + 1).join('\n');
  const tokens = fenceLexer.lexer(envelope + '\n').filter(token => token.type !== 'space');
  const tail = tokens[tokens.length - 1];
  if (
    tokens.length < 3 ||
    tokens[0].type !== 'code' ||
    tail?.type !== 'code' ||
    tail.text !== '' ||
    tokens.slice(1, -1).some(token => token.type === 'code')
  ) {
    return source;
  }

  const marker = opening[1];
  const longer = marker + marker[0];
  lines[first] = lines[first].replace(marker, longer);
  lines[last] = lines[last].replace(marker, longer);
  return lines.join('\n');
}

/** Hold only a possible fence line until its newline arrives; prose stays live. */
export function getStreamingMarkdownSource(source: string): string {
  const lineStart = source.lastIndexOf('\n') + 1;
  const tail = source.slice(lineStart).replace(/\r$/, '');
  // Include quote/list prefixes and partial markers split across stream deltas.
  const possibleFence =
    /^[ \t]*(?:>[ \t]*)*(?:(?:[-+*]|\d+[.)])[ \t]+)?(?:`{1,2}|~{1,2}|`{3,}[^\r\n]*|~{3,}[^\r\n]*)$/;
  return possibleFence.test(tail) ? source.slice(0, lineStart) : source;
}
