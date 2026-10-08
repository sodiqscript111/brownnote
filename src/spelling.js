// Offsets are UTF-16 offsets, matching textarea.selectionStart/selectionEnd.
export const normalize = word => word.normalize('NFC').replaceAll('’', "'").toLowerCase();

export function debouncer(callback, delay = 500) {
  let timer;
  return {
    schedule(...args) { clearTimeout(timer); timer = setTimeout(() => callback(...args), delay); },
    cancel() { clearTimeout(timer); },
  };
}

export function tokenize(text) {
  return Array.from(text.matchAll(/[\p{L}][\p{L}\p{M}]*(?:['’][\p{L}][\p{L}\p{M}]*)*/gu), match => ({
    word: match[0], key: normalize(match[0]), start: match.index, end: match.index + match[0].length,
  })).filter(token => [...token.key].length <= 64);
}

export class LRUCache {
  constructor(capacity = 512) {
    if (!Number.isInteger(capacity) || capacity < 1) throw new Error('Invalid LRU capacity');
    this.capacity = capacity;
    this.entries = new Map();
  }
  get(key) {
    if (!this.entries.has(key)) return undefined;
    const value = this.entries.get(key);
    this.entries.delete(key); this.entries.set(key, value);
    return value;
  }
  peek(key) { return this.entries.get(key); }
  set(key, value) {
    this.entries.delete(key);
    this.entries.set(key, value);
    if (this.entries.size > this.capacity) this.entries.delete(this.entries.keys().next().value);
  }
  get size() { return this.entries.size; }
}

export function matchCase(suggestion, original) {
  let word = suggestion;
  if (original === original.toUpperCase()) word = word.toUpperCase();
  else if (/^\p{Lu}/u.test(original)) word = word[0].toUpperCase() + word.slice(1);
  if (original.includes('’')) word = word.replaceAll("'", '’');
  return word;
}

export function correction(text, token, suggestion, selectionStart, selectionEnd) {
  if (text.slice(token.start, token.end) !== token.word) return null;
  const replacement = matchCase(suggestion, token.word);
  const delta = replacement.length - (token.end - token.start);
  const move = pos => pos < token.start ? pos : pos >= token.end ? pos + delta : token.start + replacement.length;
  return {
    text: text.slice(0, token.start) + replacement + text.slice(token.end), replacement,
    start: move(selectionStart), end: move(selectionEnd),
  };
}

export function decorations(text, results) {
  return tokenize(text).filter(token => results.get(token.key)?.correct === false)
    .map(token => ({ ...token, suggestions: results.get(token.key).suggestions }));
}

// Current-document results are transient; the reusable cross-edit cache is bounded.
export class SpellClient {
  constructor({ fetcher = (...args) => globalThis.fetch(...args), url = '/api/check', cache = new LRUCache() } = {}) {
    this.fetcher = fetcher; this.url = url; this.cache = cache; this.revision = 0;
  }
  cancel() { this.revision++; this.controller?.abort(); }
  cached(text) {
    const results = new Map();
    for (const token of tokenize(text)) {
      const result = this.cache.peek(token.key);
      if (result) results.set(token.key, result);
    }
    return results;
  }
  async check(text) {
    this.cancel();
    const revision = this.revision;
    const controller = this.controller = new AbortController();
    const keys = [...new Set(tokenize(text).map(token => token.key))];
    const results = new Map(), missing = [];
    for (const key of keys) {
      const cached = this.cache.get(key);
      if (cached) results.set(key, cached); else missing.push(key);
    }
    if (!missing.length) return { results, revision };
    if (missing.length > 4096 || new TextEncoder().encode(JSON.stringify({ words: missing })).length > 128 * 1024) {
      throw new Error('This batch exceeds the spelling limit; try a shorter document.');
    }
    let response;
    let timedOut = false;
    const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 15000);
    try {
      response = await this.fetcher(this.url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ words: missing }), signal: controller.signal });
      if (!response.ok) throw new Error('Spelling service unavailable. Your text is safe; edit to retry.');
      const data = await response.json();
      if (revision !== this.revision) return null;
      if (!Array.isArray(data.results) || data.results.length !== missing.length) throw new Error('Invalid spelling response');
      const expected = new Set(missing), received = new Set();
      for (const item of data.results) {
        if (!expected.has(item.word) || received.has(item.word) || typeof item.correct !== 'boolean' || !Array.isArray(item.suggestions) || item.suggestions.length > 5 || item.suggestions.some(s => typeof s !== 'string' || s.length > 256)) throw new Error('Invalid spelling response');
        received.add(item.word);
      }
      // Validate the entire response before touching the cache.
      for (const item of data.results) {
        this.cache.set(item.word, item); results.set(item.word, item);
      }
      return { results, revision };
    } catch (error) {
      if (revision !== this.revision) return null;
      if (timedOut) throw new Error('Spelling service timed out. Your text is safe; edit to retry.');
      if (error.name === 'AbortError') return null;
      if (error instanceof TypeError) throw new Error('Spelling service unreachable. Your text is safe; edit to retry.');
      throw error;
    } finally { clearTimeout(timeout); }
  }
}
