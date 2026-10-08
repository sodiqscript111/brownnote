<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { SpellClient, decorations, correction, matchCase, debouncer } from './spelling.js';

  const fonts = { mono: "'Courier New', monospace", serif: 'Georgia, serif', sans: 'Arial, sans-serif' };
  let text = $state('');
  let filename = $state('untitled.txt');
  let spell = $state(true);
  let font = $state('mono');
  let ready = $state(false);
  let saved = $state('● Saved on this browser');
  let cursor = $state(0);
  let editor;
  let mirror;
  let composing = $state(false);
  let results = $state(new Map());
  let popup = $state(null);
  let spellingError = $state('');
  const client = new SpellClient({ url: import.meta.env.VITE_API_URL || '/api/check' });
  const checking = debouncer(async snapshot => {
    try {
      const checked = await client.check(snapshot);
      if (checked && snapshot === text && spell && !composing) results = checked.results;
    } catch (error) {
      if (snapshot === text && spell) spellingError = error.message;
    }
  });
  const mistakes = $derived(decorations(text, results));
  const segments = $derived.by(() => {
    const parts = []; let start = 0;
    for (const token of mistakes) {
      if (token.start > start) parts.push({ text: text.slice(start, token.start), key: `text-${start}` });
      parts.push({ text: token.word, token, key: `word-${token.start}` }); start = token.end;
    }
    parts.push({ text: text.slice(start) + '\n', key: `text-${start}` });
    return parts;
  });
  const words = $derived(text.trim() ? text.trim().split(/\s+/u).length : 0);
  const before = $derived(text.slice(0, cursor));
  const line = $derived(before.split('\n').length);
  const column = $derived(before.length - before.lastIndexOf('\n'));

  onMount(() => {
    try {
      const draft = JSON.parse(localStorage.getItem('brownnote-draft'));
      if (draft) {
        text = typeof draft.text === 'string' ? draft.text : '';
        filename = typeof draft.name === 'string' ? draft.name : 'untitled.txt';
        spell = draft.spell !== false;
        font = Object.hasOwn(fonts, draft.font) ? draft.font : 'mono';
      }
    } catch {}
    ready = true;
  });

  $effect(() => {
    if (!ready) return;
    try {
      localStorage.setItem('brownnote-draft', JSON.stringify({ text, name: filename, spell, font }));
      saved = '● Saved on this browser';
    } catch {
      saved = '● Browser storage unavailable';
    }
  });

  function updateCursor(event) { cursor = event.currentTarget.selectionStart; }
  $effect(() => {
    if (!ready) return;
    const snapshot = text, enabled = spell, isComposing = composing;
    checking.cancel(); client.cancel(); popup = null;
    results = enabled ? client.cached(snapshot) : new Map();
    spellingError = '';
    if (!enabled || isComposing) return;
    checking.schedule(snapshot);
  });
  onDestroy(() => { checking.cancel(); client.cancel(); });

  $effect(() => {
    text; font; results;
    tick().then(() => {
      if (mirror && editor) { mirror.scrollTop = editor.scrollTop; mirror.scrollLeft = editor.scrollLeft; }
    });
  });

  function syncScroll() {
    if (mirror && editor) { mirror.scrollTop = editor.scrollTop; mirror.scrollLeft = editor.scrollLeft; }
    popup = null;
  }
  function showPopup(token, x, y) {
    popup = { token, text, selectionStart: editor.selectionStart, selectionEnd: editor.selectionEnd,
      x: Math.max(8, Math.min(x, window.innerWidth - 238)), y: Math.max(8, Math.min(y + 8, window.innerHeight - 230)) };
  }
  function clicked(event) {
    updateCursor(event); popup = null;
    if (!spell || composing || editor.selectionStart !== editor.selectionEnd) return;
    for (const mark of mirror.querySelectorAll('.misspelled')) {
      if ([...mark.getClientRects()].some(r => event.clientX >= r.left && event.clientX <= r.right && event.clientY >= r.top && event.clientY <= r.bottom)) {
        const token = mistakes.find(t => t.start === Number(mark.dataset.start));
        if (token) showPopup(token, event.clientX, event.clientY);
        break;
      }
    }
  }
  function keydown(event) {
    if (event.key === 'Escape') popup = null;
    if (event.altKey && event.key === 'Enter') {
      const token = mistakes.find(t => editor.selectionStart >= t.start && editor.selectionStart <= t.end);
      if (token) {
        event.preventDefault();
        const rect = mirror.querySelector(`[data-start="${token.start}"]`)?.getBoundingClientRect();
        if (rect) { showPopup(token, rect.left, rect.bottom); tick().then(() => document.querySelector('.suggestion-popup button')?.focus()); }
      }
    }
  }
  async function applySuggestion(suggestion) {
    if (!popup || popup.text !== text) { popup = null; return; }
    const next = correction(text, popup.token, suggestion, popup.selectionStart, popup.selectionEnd);
    if (!next) { popup = null; return; }
    const scrollTop = editor.scrollTop, scrollLeft = editor.scrollLeft;
    editor.setRangeText(next.replacement, popup.token.start, popup.token.end, 'preserve');
    text = editor.value; popup = null;
    await tick();
    editor.focus(); editor.setSelectionRange(next.start, next.end);
    cursor = next.start; editor.scrollTop = scrollTop; editor.scrollLeft = scrollLeft; syncScroll();
  }
</script>

<svelte:window onresize={() => popup = null} onscroll={() => popup = null}
  onkeydown={event => { if (event.key === 'Escape' && popup) { popup = null; editor.focus(); } }}
  onpointerdown={event => { if (!event.target.closest('.suggestion-popup') && event.target !== editor) popup = null; }} />

<section class="window" aria-label="Text editor">
  <div class="titlebar">
    <span>▤ &nbsp; Brownnote <span class="titlefile">/ {filename || 'untitled.txt'}</span></span>
    <span class="windowdots" aria-hidden="true">― &nbsp; □</span>
  </div>
  <div class="toolbar">
    {#if spellingError}<span class="spelling-error" role="status">{spellingError}</span>{/if}
    <span class="spacer"></span>
    <label class="spell"><input type="checkbox" bind:checked={spell} onchange={() => editor?.focus()}> Spell check</label>
    <select aria-label="Editor font" bind:value={font}>
      <option value="mono">Monospace</option><option value="serif">Serif</option><option value="sans">Sans serif</option>
    </select>
  </div>
  <div class="documentbar">
    <label><span class="fileicon">▤</span><input bind:value={filename} aria-label="File name" maxlength="100"></label>
    <span>{saved}</span>
  </div>
  <div class="writing">
    <div class="margin" aria-hidden="true">01</div>
    <div class="editor-surface" style:font-family={fonts[font]}>
      <div class="spelling-mirror" bind:this={mirror} aria-hidden="true">{#each segments as segment (segment.key)}{#if segment.token}<span class="misspelled" data-start={segment.token.start}>{segment.text}</span>{:else}{segment.text}{/if}{/each}</div>
      <textarea bind:this={editor} bind:value={text} spellcheck={false} lang="en" aria-label="Document text"
        style:font-family={fonts[font]} oninput={updateCursor} onkeyup={updateCursor} onclick={clicked} onselect={updateCursor} onkeydown={keydown} onscroll={syncScroll}
        oncompositionstart={() => composing = true} oncompositionend={() => composing = false}
        placeholder="Every good idea starts somewhere.&#10;&#10;Start typing..."></textarea>
    </div>
  </div>
  <div class="statusbar">
    <span>{words} words &nbsp; | &nbsp; {text.length} characters</span>
    <span>Ln {line}, Col {column}</span><span>UTF-8 <i>•</i> PLAIN TEXT</span>
  </div>
</section>

{#if popup}
  <div class="suggestion-popup" role="dialog" aria-label={`Spelling suggestions for ${popup.token.word}`}
    style:left={`${popup.x}px`} style:top={`${popup.y}px`}>
    <div class="suggestion-title">{popup.token.word}<button aria-label="Close suggestions" onclick={() => { popup = null; editor.focus(); }}>×</button></div>
    {#each popup.token.suggestions as suggestion}
      <button class="suggestion" onclick={() => applySuggestion(suggestion)}>{matchCase(suggestion, popup.token.word)}</button>
    {:else}<p>No suggestions found.</p>{/each}
  </div>
{/if}
