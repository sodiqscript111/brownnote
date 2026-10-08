# Brownnote

A Svelte 5 text editor with a Go spelling API using Gin v1.12.0. The existing brown UI and page layout are preserved. No database, Redis, accounts, or server-side document storage.

Read [What I implemented](PROJECT_DECISIONS.md) for a short list of the editor, caching and reliability work.

## Run locally

Requirements: Node 22.12+ (or 20.19+) and Go 1.25.3+. Normal builds need no C compiler or native Hunspell installation. The race detector requires a supported C compiler.

From this project folder:

```sh
npm install
npm run build
cd backend
go run .
```

Open **http://127.0.0.1:8080**. Go serves both `../dist` and `/api/check`; requests are same-origin. The dictionary is bundled and embedded in the executable and needs no runtime download. Stop with Ctrl+C.

For development, leave Go running and run `npm run dev` in a second terminal at the project root. Vite normally serves http://127.0.0.1:5173 and proxies `/api` to port 8080. `npm run preview` has the same proxy.

Edit `src/`, not generated `dist/`. Rebuild to update the Go-served frontend.

For a standalone Windows server, run `go build -o brownnote.exe .` inside `backend`, then `./brownnote.exe -addr 127.0.0.1:8080 -static ../dist`. On other platforms use `go build -o brownnote .` and `./brownnote`. Dictionary files are embedded; keep the built frontend at the configured static path.

If this laptop's npm launcher reports a missing npm-cli.js, use `node "C:/Program Files/nodejs/node_modules/npm/bin/npm-cli.js"` in place of `npm` in the commands above.

## Usage

Type, pause 500ms, then click a red underlined word. Choose a suggestion to replace only that occurrence. Keyboard: put the caret inside the word, press **Alt+Enter**, then Tab/Enter to select a suggestion. Escape dismisses it. Uppercase/initial-uppercase casing and curly apostrophes are preserved. The spell-check toggle cancels checking and clears underlines; font selection still works.

Drafts remain in this browser through the original localStorage key. The reusable spelling cache is in memory and resets on reload. On network failure, a small message appears; typing and local draft saving continue. Edit or toggle checking off/on to retry.

## Architecture and code

| File | Responsibility |
|---|---|
| `src/App.svelte` | Existing page shell |
| `src/Editor.svelte` | Textarea, debounce, underline mirror, popup, corrections and selection |
| `src/spelling.js` | Positioned tokens, normalization, bounded LRU, batched requests, stale-response protection |
| `src/resilience.js` | Abortable delays, request deadlines and circuit breaker |
| `src/style.css` | Existing appearance plus underline and popup styles |
| `backend/main.go` | Application startup and graceful shutdown |
| `backend/config.go` | Command-line settings, request limits and server timeouts |
| `backend/router.go` | Gin routes, recovery middleware and static files |
| `backend/handler.go` | Request validation and batch checking |
| `backend/response.go` | Shared JSON response headers and error responses |
| `backend/checker.go` | Dictionary loading, normalization and spelling suggestions |
| `backend/dictionary/` | Pinned English dictionary, source and license notices |

Text remains in a native textarea. A non-interactive mirror underneath paints transparent text with red wavy underlines. Both layers share font, wrapping, padding, width and scroll offsets. Svelte updates keyed mirror segments without replacing the editable DOM, so asynchronous spelling results do not move the caret or change selection. Clicks are handled by the textarea against visible mirror word rectangles. `setRangeText` applies corrections, then the adjusted selection and scroll position are restored.

Tokens retain UTF-16 offsets, matching textarea selection positions. The tokenizer handles Unicode letters, combining marks and straight/curly apostrophes, preserving punctuation and original case. Hyphenated words are checked as separate components. Numbers and tokens longer than 64 code points are skipped. Keys use NFC, lowercase and straight apostrophes. Text is never inserted as raw HTML.

The backend uses [client9/gospell v0.9.2](https://github.com/client9/gospell), a pure-Go Hunspell dictionary reader. `NewGoSpellReader`, `Spell` and `Suggest(word, 5)` were verified through source inspection and `go doc` before integration. The US English dictionary is pinned to the commit in `backend/dictionary/SOURCE.txt`; its upstream license is included.

The dictionary loads once at startup. Queries try normalized lowercase and uppercase forms to recognize proper nouns and `I` contractions case-insensitively. A mutex protects each validation/suggestion operation because lazy dictionary lookups mutate internal maps. Gin handles routing and JSON responses on a standard `net/http.Server`, preserving explicit timeouts and graceful shutdown. There are no per-word goroutines or worker pools. The server has no application response cache or document state; the library retains its own lazy dictionary surfaces.

The router uses `gin.New()` with recovery middleware returning a generic JSON 500 on panics. Strict JSON decoding remains explicit to reject unknown fields and trailing content while enforcing body limits. Unsupported methods on `/api/check` return JSON 405 with `Allow: POST`; unknown API routes return JSON 404. Static frontend files use the standard file server through Gin's fallback handler. Automatic path redirects and trusted proxy handling are disabled. Gin adds framework dependencies, but does not change the API contract or client code.

## API

```http
POST /api/check
Content-Type: application/json

{"words":["HELLO","helo","helo","don’t"]}
```

Example response (ranking depends on the pinned library):

```json
{"results":[
  {"word":"hello","correct":true,"suggestions":[]},
  {"word":"helo","correct":false,"suggestions":["hell","help","helot","hello","held"]},
  {"word":"don't","correct":true,"suggestions":[]}
]}
```

The server independently normalizes and deduplicates inputs. Results follow first normalized occurrence order; empty arrays return an empty result array. Correct words have no suggestions.

Limits: **128 KiB body**, **4096 words**, **64 code points per word**, **five suggestions**. Words must contain letters with optional combining marks and internal apostrophes. Missing/non-array words, punctuation inside a word, unknown fields, malformed or trailing JSON are rejected. JSON errors: 400 invalid input, 405 wrong method, 413 body too large, 415 wrong content type, 500 checker failure. Unknown `/api/` paths return 404. Read/header/write/idle timeouts are explicit; cancelled requests stop between words.

The browser also checks batch limits, enforces a 15-second deadline per attempt (including response-body reading), and validates the entire response before caching. Same-origin hosting is the default. `VITE_API_URL` can override the endpoint at build time; a separate origin requires deliberately configured reverse proxy/CORS handling, which is not enabled here.

## Retries, circuit breaker and dependency injection

Network failures, timeouts and HTTP 502/503/504 get at most two retries, with delays of about one and two seconds and ±20% jitter. Other HTTP errors and malformed responses are not retried. Checking is read-only, so repeating a batch is safe. Worst-case duration is about 49 seconds for three timed-out attempts; editing, disabling checking or leaving the editor cancels both requests and retry delays immediately.

After three consecutive checking operations exhaust their retries, the client circuit opens for 30 seconds. This pauses uncached network checks; it does not interrupt editing or discard cached spelling results. The next edit after cooldown permits one recovery attempt, without retries. Success closes the circuit; another temporary failure restarts cooldown. Cancelled work never counts as a failure. Recovery is triggered by an edit or toggling checking, not a background polling loop.

Dependency injection stays small: `SpellClient` accepts fetch, cache, timeout, retry delays, random source, sleep and breaker dependencies. `CircuitBreaker` accepts a clock. Go's HTTP handler accepts a `spellChecker` interface; the production checker loads once and protects the library with a mutex. Tests can substitute these dependencies without real delays or dictionary failures. There is no DI container or resilience package. The breaker belongs in the browser-to-API boundary because the Go backend has no remote dependency to retry; server request limits and timeouts remain in place.

## Why debounce, batching and LRU

**500ms debounce** waits until typing stops before network work. Edits immediately cancel older requests. IME composition suspends checking until it finishes, and incomplete words are not sent while typing continues. AbortController plus revisions prevent late responses from applying even when a network ignores cancellation.

**Batching** sends one request for unique uncached words, not one request per occurrence. Repeated words share validation but keep individual positions. Adding one word only requests that word when the paragraph's other words are cached.

**512-entry Map LRU** bounds reusable results, including suggestions. Hits refresh recency; inserting at capacity evicts the oldest key. Positions never enter the cache, and all-cached documents cause no request. A transient current-document result map allows a successful batch larger than 512 entries to be fully decorated. On later edits, evicted words need another request; this is the deliberate memory trade-off.

## Tests and benchmarks

```sh
npm test
npm run build
cd backend
go test ./...
go test -race ./...
go vet ./...
go test -bench Benchmark -benchmem -run NotATest -benchtime=100ms
```

JavaScript tests cover debounce/cancellation, LRU eviction/refresh/reuse, native fetch, repeated words, punctuation/contractions/case, Unicode offsets, individual corrections, insertion/deletion, stale responses, invalid responses, failures and batch limits. Resilience tests cover retry limits/jitter, non-retryable errors, cancellation during waits, strict deadlines, circuit cooldown, single recovery probes and cached checks while open. Go tests exercise the actual embedded dictionary, suggestions, deduplication, input/size validation, concurrent requests injected checker failures, Gin routing/static files, cancellation and panic recovery.

Browser checks against the real Go server cover red underlines, clicked suggestions, correcting one repeated occurrence, adjusted caret position, fonts, keyboard correction, scrolling and the spell toggle. Representative warm-library benchmarks on this laptop were about **408ns per validation** and **3.6ms per suggestion query** for `hello`/`helo`, not latency guarantees for every word or batch.

## Trade-offs and deliberately excluded features

- Dictionary spelling is not contextual grammar. US English only; names, jargon and other languages may be flagged. Hunspell-format compatibility does not reproduce every native Hunspell feature or ranking.
- Tokenization/mirror segmentation are linear in document length. For small notes this keeps the code simple; very large documents merit incremental tokenization and viewport decoration. The editable textarea itself stays intact.
- Programmatic corrections use `setRangeText`; browser undo behavior for these corrections varies. Normal typing and selection remain native. Explicit correction undo/redo is a future improvement.
- No Count-Min Sketch, TinyLFU, Redis, distributed caching, database, personal dictionary, grammar engine, automatic corrections, rich text, accounts or new frontend framework.
- No authentication, public CORS or rate limiter. For public hosting, add appropriate controls at the serving boundary and profile contention before replacing the mutex.
- Current ChatGPT Sites hosting cannot execute Go. The full app runs locally or on a Go-capable host with the binary and `dist`. The hosted frontend requires a reachable Go API/reverse proxy; publishing frontend assets does not deploy the Go service.

With more time: broader browser/IME automation, reliable correction undo history, dictionary/language improvements and personal allowlists, incremental handling of large documents, performance profiling and production Go hosting.
