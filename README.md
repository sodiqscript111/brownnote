# Brownnote

A simple text editor with inline spell checking, built with Svelte and Go/Gin. Spelling checks use a bundled English dictionary—your text is not sent to an external service.

## Run on macOS

From the project folder:

```sh
bash scripts/start-macos.sh
```

Installs missing tools and dependencies, builds the app and opens the editor. Homebrew may ask for your Mac password. The launcher was tested with simulated commands; a real Mac install still needs verification.

## Run manually

Requires Node.js 22.12+ and Go 1.25.3+.

```sh
npm ci
npm run build
cd backend
go run .
```

Open **http://127.0.0.1:8080**. Press **Ctrl+C** to stop.

For frontend development, keep Go running and run `npm run dev` in a second terminal at the project root.

## Use the editor

- Type, pause, then click a red underlined word to see corrections.
- Selecting a suggestion replaces only that occurrence.
- Drafts are saved in your browser.
- Checked words are cached, so edits only send uncached words to the backend.

The frontend lives in `src/`; the Go backend lives in `backend/`. See [What I implemented](PROJECT_DECISIONS.md) for the implementation details.

## API

`POST /api/check` accepts `{"words":["hello","helo"]}` and returns a `results` array with `word`, `correct` and up to five `suggestions` per word.

Limits: 128 KiB per request, 4,096 words per batch and 64 Unicode code points per word. Spelling uses [gospell](https://github.com/client9/gospell) with a bundled US English dictionary. It does not check grammar.

## Tests

```sh
npm test
npm run test:launcher
cd backend
go test ./...
go test -race ./...
```

The Go service runs locally or on a Go-capable host.
