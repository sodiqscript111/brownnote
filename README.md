# Brownnote

A small text editor built with Svelte 5 and Vite. The backend can be connected through API calls in the editor component when its endpoints are ready.

## Edit the code

- `src/App.svelte`: page layout, header, and footer.
- `src/Editor.svelte`: editor controls, reactive state, word counts, and local draft storage.
- `src/style.css`: colors, spacing, and responsive styles.
- `src/main.js`: mounts the Svelte application.
- `index.html`: browser entry page and metadata.

From this folder, run `npm install` once, then `npm run dev`. Open the local URL shown in the terminal. Saving source changes updates the preview automatically.

Run `npm run build` to generate the production site in `dist/`, then `npm run preview` to preview that build. Edit `src/` rather than generated files in `dist/`. Spell-check suggestions depend on your browser's spelling settings.

If this laptop's npm launcher reports a missing npm-cli.js, use `node "C:/Program Files/nodejs/node_modules/npm/bin/npm-cli.js"` in place of `npm` in the commands above.

The hosted version is https://brownnote-editor.citrusmesa4.chatgpt.site/. Local edits do not automatically update the hosted version; ask Codex to publish them when ready.
