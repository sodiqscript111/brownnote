import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, copyFileSync, writeFileSync, readFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const bash = process.env.BROWNNOTE_TEST_BASH || (process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : '/bin/bash');
const launcher = fileURLToPath(new URL('./start-macos.sh', import.meta.url));
const shellPath = value => process.platform === 'win32' ? value.replaceAll('\\', '/').replace(/^([A-Za-z]):/, (_, drive) => `/${drive.toLowerCase()}`) : value;

function run(overrides = {}) {
  const fixture = mkdtempSync(path.join(tmpdir(), 'brownnote-launcher-'));
  const project = path.join(fixture, 'project with spaces');
  const bin = path.join(fixture, 'bin');
  mkdirSync(bin);
  mkdirSync(path.join(project, 'scripts'), { recursive: true });
  mkdirSync(path.join(project, 'backend'));
  copyFileSync(launcher, path.join(project, 'scripts/start-macos.sh'));
  writeFileSync(path.join(project, 'package-lock.json'), '{}');
  writeFileSync(path.join(project, 'backend/go.mod'), 'module brownnote\n');
  const mocks = {
    uname: 'printf "%s\\n" "${BROWNNOTE_TEST_OS:-Darwin}"',
    lsof: '[[ "${BROWNNOTE_TEST_BUSY:-0}" == 1 ]]',
    node: '[[ "${BROWNNOTE_TEST_OLD_NODE:-0}" != 1 || -f "$BROWNNOTE_INSTALLED_NODE" ]]',
    npm: `printf 'npm %s\\n' "$*" >> "$BROWNNOTE_TEST_LOG"
[[ "\${BROWNNOTE_TEST_FAIL_NPM:-0}" != 1 ]] || exit 7
mkdir -p dist`,
    go: `if [[ "$1" == env ]]; then
  if [[ "\${BROWNNOTE_TEST_OLD_GO:-0}" == 1 && ! -f "$BROWNNOTE_INSTALLED_GO" ]]; then printf 'go1.24.0\\n'; else printf 'go1.25.3\\n'; fi
else
  printf 'go %s\\n' "$*" >> "$BROWNNOTE_TEST_LOG"
  if [[ "$1" == build ]]; then
    [[ "\${BROWNNOTE_TEST_FAIL_GO:-0}" != 1 ]] || exit 8
    printf '#!/bin/bash\\nexec sleep 1\\n' > "$3"
    chmod +x "$3"
  fi
fi`,
    brew: `case "$1" in
  shellenv) printf '\\n' ;;
  --prefix) printf '%s\\n' "$BROWNNOTE_TEST_ROOT" ;;
  install)
    printf 'brew install %s\\n' "$2" >> "$BROWNNOTE_TEST_LOG"
    if [[ "$2" == node@24 ]]; then touch "$BROWNNOTE_INSTALLED_NODE"; else touch "$BROWNNOTE_INSTALLED_GO"; fi ;;
esac`,
    curl: 'printf "curl %s\\n" "$*" >> "$BROWNNOTE_TEST_LOG"',
    open: 'printf "open %s\\n" "$*" >> "$BROWNNOTE_TEST_LOG"; [[ "${BROWNNOTE_TEST_FAIL_OPEN:-0}" != 1 ]]',
  };
  for (const [name, body] of Object.entries(mocks)) writeFileSync(path.join(bin, name), `#!/bin/bash\n${body}\n`, { mode: 0o755 });
  const logfile = path.join(fixture, 'calls.log');
  const inherited = Object.fromEntries(Object.entries(process.env).filter(([key]) => key.toLowerCase() !== 'path'));
  try {
    const result = spawnSync(bash, ['-c', 'export PATH="$1:/usr/bin:/bin"; exec /bin/bash scripts/start-macos.sh', 'brownnote-test', shellPath(bin)], {
      cwd: project, encoding: 'utf8', timeout: 15000,
      env: { ...inherited, PATH: `${shellPath(bin)}:/usr/bin:/bin`, BROWNNOTE_TEST_ROOT: shellPath(fixture),
        BROWNNOTE_TEST_LOG: shellPath(logfile), BROWNNOTE_INSTALLED_NODE: shellPath(path.join(fixture, 'node-installed')),
        BROWNNOTE_INSTALLED_GO: shellPath(path.join(fixture, 'go-installed')), BROWNNOTE_PORT: '8080', ...overrides },
    });
    if (result.error) throw result.error;
    return { ...result, calls: existsSync(logfile) ? readFileSync(logfile, 'utf8') : '' };
  } finally {
    assert.equal(path.dirname(fixture), tmpdir());
    assert.ok(path.basename(fixture).startsWith('brownnote-launcher-'));
    rmSync(fixture, { recursive: true, force: true });
  }
}

test('macOS launcher builds and opens the editor with existing tools and paths containing spaces', () => {
  const result = run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.calls, /npm ci --no-audit --no-fund/);
  assert.match(result.calls, /npm run build/);
  assert.match(result.calls, /go mod download/);
  assert.match(result.calls, /go build -o .*project with spaces.*\.local\/brownnote \./);
  assert.match(result.calls, /open http:\/\/127\.0\.0\.1:8080/);
  assert.doesNotMatch(result.calls, /brew install/);
});

test('macOS launcher installs replacements for incompatible Node and Go', () => {
  const result = run({ BROWNNOTE_TEST_OLD_NODE: '1', BROWNNOTE_TEST_OLD_GO: '1' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.calls, /brew install node@24/);
  assert.match(result.calls, /brew install go/);
  assert.match(result.calls, /open http/);
});

test('macOS launcher rejects other operating systems before installation', () => {
  const result = run({ BROWNNOTE_TEST_OS: 'Linux' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /for macOS/);
  assert.equal(result.calls, '');
});

test('macOS launcher rejects occupied and invalid ports before installation', () => {
  const busy = run({ BROWNNOTE_TEST_BUSY: '1' });
  assert.equal(busy.status, 1);
  assert.match(busy.stderr, /already in use/);
  assert.equal(busy.calls, '');
  for (const port of ['0', '65536', 'oops']) {
    const result = run({ BROWNNOTE_PORT: port });
    assert.equal(result.status, 1);
    assert.equal(result.calls, '');
  }
});

test('macOS launcher does not start the server after dependency or build failure', () => {
  for (const setting of ['BROWNNOTE_TEST_FAIL_NPM', 'BROWNNOTE_TEST_FAIL_GO']) {
    const result = run({ [setting]: '1' });
    assert.notEqual(result.status, 0);
    assert.doesNotMatch(result.calls, /open http/);
    assert.doesNotMatch(result.calls, /curl /);
  }
});

test('macOS launcher accepts a custom port and handles browser-open failure', () => {
  const result = run({ BROWNNOTE_PORT: '08081', BROWNNOTE_TEST_FAIL_OPEN: '1' });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.calls, /open http:\/\/127\.0\.0\.1:8081/);
  assert.match(result.stdout, /Open the address above/);
});
