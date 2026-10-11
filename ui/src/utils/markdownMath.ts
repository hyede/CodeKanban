import katex from 'katex';
import type { MarkedExtension, Tokens } from 'marked';

interface MathToken extends Tokens.Generic {
  text: string;
  displayMode: boolean;
  complete: boolean;
}

function findClosingDelimiter(source: string, start: number, delimiter: string) {
  let depth = 0;
  for (let index = start; index < source.length; index += 1) {
    const char = source[index];
    if (depth === 0 && source.startsWith(delimiter, index)) {
      return index;
    }
    if (char === '\\') {
      index += 1;
    } else if (char === '{') {
      depth += 1;
    } else if (char === '}') {
      depth = Math.max(0, depth - 1);
    }
  }
  return -1;
}

function tokenizeMath(source: string, block: boolean): MathToken | undefined {
  const opening = block
    ? /^( {0,3})(\$\$|\\\[)/.exec(source)
    : /^()(\$\$|\$|\\\(|\\\[)/.exec(source);
  if (!opening) {
    return;
  }

  const delimiter = opening[2]!;
  const closing = delimiter === '\\[' ? '\\]' : delimiter === '\\(' ? '\\)' : delimiter;
  const start = opening[0].length;
  const end = findClosingDelimiter(source, start, closing);
  const displayMode = delimiter === '$$' || delimiter === '\\[';

  // Dollars also occur in prices: disallow whitespace just inside the pair
  // and a digit immediately after the closing dollar.
  if (delimiter === '$') {
    if (
      end < 0 ||
      /\s/.test(source[start] ?? '') ||
      /\s/.test(source[end - 1] ?? '') ||
      /[\d$]/.test(source[end + 1] ?? '') ||
      source.slice(start, end).includes('\n')
    ) {
      return;
    }
  }

  // Keep unfinished display math together while a response is streaming.
  if (end < 0 && !block) {
    return;
  }
  const raw = end < 0 ? source : source.slice(0, end + closing.length);
  return {
    type: block ? 'blockMath' : 'inlineMath',
    raw,
    text: source.slice(start, end < 0 ? undefined : end).trim(),
    displayMode,
    complete: end >= 0,
  };
}

function renderMath(token: MathToken, block: boolean) {
  let html: string | undefined;
  if (token.complete) {
    try {
      html = katex.renderToString(token.text, {
        displayMode: token.displayMode,
        throwOnError: true,
        trust: false,
        strict: 'ignore',
        maxExpand: 1000,
        maxSize: 20,
      });
    } catch {
      // Unsupported TeX stays readable without breaking the rest of Markdown.
    }
  }
  if (html === undefined) {
    const escaped = token.raw
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
    html = '<span class="markdown-math-source">' + escaped + '</span>';
  }
  const tag = block ? 'div' : 'span';
  const displayClass = token.displayMode ? ' markdown-math-block' : '';
  return (
    '<' +
    tag +
    ' class="markdown-math' +
    displayClass +
    '" data-markdown-math="true">' +
    html +
    '</' +
    tag +
    '>' +
    (block ? '\n' : '')
  );
}

export function createMarkdownMathExtension(): MarkedExtension {
  return {
    extensions: [
      {
        name: 'blockMath',
        level: 'block',
        start(source) {
          return /(?:^|\n) {0,3}(?:\$\$|\\\[)/.exec(source)?.index;
        },
        tokenizer(source) {
          return tokenizeMath(source, true);
        },
        renderer(token) {
          return renderMath(token as MathToken, true);
        },
      },
      {
        name: 'inlineMath',
        level: 'inline',
        start(source) {
          return /\$|\\[([]/.exec(source)?.index;
        },
        tokenizer(source) {
          return tokenizeMath(source, false);
        },
        renderer(token) {
          return renderMath(token as MathToken, false);
        },
      },
    ],
  };
}
