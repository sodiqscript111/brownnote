# Brownnote: project decisions

Brownnote is a small text editor with inline English spelling checks. The aim was to keep the existing brown, early-2000s appearance while making typing, checking and corrections reliable. The implementation favors a working end-to-end application and understandable code over infrastructure that a small editor does not need.

## Preserving the frontend

Svelte was already selected for the project, so the implementation reused its components and styling. The spelling work extends the existing editor rather than replacing the interface or introducing another framework.

A native textarea keeps text entry, selection, keyboard navigation and IME composition under the browser's control. A transparent text mirror draws red wavy underlines at the same positions, and a small popup offers corrections. Updating spelling results leaves the editable element intact, helping prevent cursor jumps during asynchronous checks.

This is practical for short notes. Keeping the mirror aligned requires matching fonts, wrapping and scrolling. Larger documents would benefit from incremental tokenization and rendering only visible decorations. Applying corrections uses `setRangeText`; correction undo behavior can vary between browsers.

## Checking only words that need checking

The client waits for a 500ms typing pause, tokenizes the current text, normalizes word keys and removes duplicates. It compares those keys with a 512-entry LRU cache and sends only missing words in one batch.

For example, after a 200-word paragraph has been checked, changing one word sends only the edited word if the other results remain cached. If the edited word is also cached, there is no request. The client still scans the text locally to reconstruct positions; reducing network work does not mean that local parsing is incremental.

The cache stores validation and suggestions, never document offsets. This lets repeated words share a result while preserving separate clickable occurrences. A correction replaces only the selected occurrence, and subsequent tokenization locates underlines after insertions and deletions.

Using a JavaScript `Map` keeps the cache easy to inspect: reads refresh recency and adding an entry beyond capacity removes the oldest one. The bound prevents memory growth across long editing sessions. The trade-off is that evicted words need checking again, and reloading the page clears this in-memory cache.

## A small Go API

Go's standard `net/http` package provides the required routing, concurrent request handling, limits and timeouts without another server framework. `POST /api/check` accepts a batch of words and returns spelling results with up to five suggestions per incorrect word.

The implementation uses the pure-Go `client9/gospell` library with a bundled US English Hunspell-format dictionary. The library API was verified before integration. Embedding dictionary files makes the executable self-contained and avoids runtime downloads or a native Hunspell installation.

The dictionary loads once at startup. A mutex protects library operations because its lazy lookups can mutate internal maps. Requests can arrive concurrently, while dictionary access is serialized safely. This is a straightforward choice for a small application; throughput profiling should precede any more complex concurrency design.

Both client and server normalize words and deduplicate batches. The server independently enforces input and body limits because browser validation cannot be trusted as the only boundary. Documents are not stored on the server. Browser drafts use localStorage, so text sent for checking and locally saved drafts have different lifetimes.

## Keeping editing responsive during failures

Every edit invalidates the previous checking operation. AbortController cancels requests and retry delays, while revision tracking prevents late responses from changing the current document or cache. Response validation happens before results enter the cache.

Temporary network failures, timeouts and HTTP 502/503/504 receive at most two retries, spaced about one and two seconds apart with jitter. Validation errors and malformed responses are not retried. Each attempt has a 15-second deadline, including reading the response body. A slow service can therefore take about 49 seconds to exhaust all attempts, but typing continues and an edit cancels the operation.

After three consecutive checking operations exhaust their retries, a client circuit breaker pauses uncached checks for 30 seconds. Cached results remain available. The next edit after the pause permits one recovery attempt; success closes the circuit and a temporary failure restarts the pause. There is no background recovery polling. The breaker pauses service calls rather than breaking the editor.

The Go backend has no remote service dependency, so adding backend retries or another circuit breaker would not solve a current failure mode. Its request limits and server timeouts provide the relevant protection.

## Small dependency boundaries

The Go HTTP handler accepts a checker interface. The client accepts fetch, cache and timing dependencies, and the breaker accepts a clock. These boundaries let tests simulate failures, cancellation and cooldowns without waiting in real time or altering the real dictionary.

There is no dependency injection container. Constructors and one small interface provide the isolation this project needs with less setup and fewer dependencies.

## Verification and scope

Frontend tests exercise caching, batches, positions, individual corrections, stale responses, retry limits and circuit recovery. Go tests use the embedded dictionary for validation and suggestions, and injected checkers for failure handling. Race checks and builds help verify that the application can run safely and compile as delivered. The README contains reproducible commands.

Deliberately excluded features include databases, Redis, distributed caching, TinyLFU, Count-Min Sketch, accounts, rich text and contextual grammar checking. The application checks US English spelling; dictionary suggestions do not understand sentence meaning and may flag names or specialist vocabulary.

The published Sites frontend does not deploy the Go process. Full functionality runs locally or on a Go-capable host serving the built frontend and API. Public production hosting would also need serving-boundary controls appropriate to its audience.

With more time, the most useful improvements would be broader browser and IME tests, dependable correction undo history, support for other dictionaries and personal allowlists, large-document profiling, and a production Go deployment.
