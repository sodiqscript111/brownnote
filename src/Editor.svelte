<script>
  import { onMount } from 'svelte';

  const fonts = { mono: "'Courier New', monospace", serif: 'Georgia, serif', sans: 'Arial, sans-serif' };
  let text = $state('');
  let filename = $state('untitled.txt');
  let spell = $state(true);
  let font = $state('mono');
  let ready = $state(false);
  let saved = $state('● Saved on this browser');
  let cursor = $state(0);
  let editor;
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
    } catch { /* An unavailable or invalid draft leaves a fresh document. */ }
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
</script>

<section class="window" aria-label="Text editor">
  <div class="titlebar">
    <span>▤ &nbsp; Brownnote <span class="titlefile">/ {filename || 'untitled.txt'}</span></span>
    <span class="windowdots" aria-hidden="true">― &nbsp; □</span>
  </div>
  <div class="toolbar">
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
    <textarea bind:this={editor} bind:value={text} spellcheck={spell} lang="en" aria-label="Document text"
      style:font-family={fonts[font]} oninput={updateCursor} onkeyup={updateCursor} onclick={updateCursor} onselect={updateCursor}
      placeholder="Every good idea starts somewhere.&#10;&#10;Start typing..."></textarea>
  </div>
  <div class="statusbar">
    <span>{words} words &nbsp; | &nbsp; {text.length} characters</span>
    <span>Ln {line}, Col {column}</span><span>UTF-8 <i>•</i> PLAIN TEXT</span>
  </div>
</section>
