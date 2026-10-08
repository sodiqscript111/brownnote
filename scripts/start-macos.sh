#!/bin/bash
set -euo pipefail

die() { printf 'Brownnote: %s\n' "$*" >&2; exit 1; }

[[ "$(uname -s)" == Darwin ]] || die 'This launcher is for macOS.'
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
[[ -f package-lock.json && -f backend/go.mod ]] || die 'Run this from a complete Brownnote project download.'

server_pid=''
installer_file=''
cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  if [[ -n "$installer_file" ]]; then rm -f "$installer_file"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

port="${BROWNNOTE_PORT:-8080}"
[[ "$port" =~ ^[0-9]{1,5}$ ]] || die 'BROWNNOTE_PORT must be a number from 1 to 65535.'
port=$((10#$port))
(( port >= 1 && port <= 65535 )) || die 'BROWNNOTE_PORT must be from 1 to 65535.'
if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
  die "Port $port is already in use. Try BROWNNOTE_PORT=8081 bash scripts/start-macos.sh"
fi

node_ready() {
  command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 &&
    node -e 'const [a,b]=process.versions.node.split(".").map(Number); process.exit((a===20&&b>=19)||(a===22&&b>=12)||a>22 ? 0 : 1)' >/dev/null 2>&1
}

go_ready() {
  command -v go >/dev/null 2>&1 || return 1
  local version major minor patch
  version="$(go env GOVERSION)" || return 1
  [[ "$version" =~ ^go([0-9]+)\.([0-9]+)(\.([0-9]+))? ]] || return 1
  major="${BASH_REMATCH[1]}"; minor="${BASH_REMATCH[2]}"; patch="${BASH_REMATCH[4]:-0}"
  (( major > 1 || (major == 1 && (minor > 25 || (minor == 25 && patch >= 3))) ))
}

ensure_brew() {
  if ! command -v brew >/dev/null 2>&1; then
    for location in /opt/homebrew/bin/brew /usr/local/bin/brew; do
      if [[ -x "$location" ]]; then export PATH="$(dirname "$location"):$PATH"; break; fi
    done
  fi
  if ! command -v brew >/dev/null 2>&1; then
    printf 'Installing Homebrew. Its installer may ask for your Mac password or developer tools.\n'
    installer_file="$(mktemp -t brownnote-homebrew)"
    curl --fail --show-error --location https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh --output "$installer_file"
    /bin/bash "$installer_file"
    for location in /opt/homebrew/bin/brew /usr/local/bin/brew; do
      if [[ -x "$location" ]]; then export PATH="$(dirname "$location"):$PATH"; break; fi
    done
  fi
  command -v brew >/dev/null 2>&1 || die 'Homebrew installation did not complete. Resolve the installer error and run this command again.'
  eval "$(brew shellenv)"
}

if ! node_ready; then
  ensure_brew
  printf 'Installing Node.js 24 and npm...\n'
  brew install node@24
  export PATH="$(brew --prefix node@24)/bin:$PATH"
  hash -r
  node_ready || die 'Node.js installation did not provide a compatible Node/npm. Update node@24 with Homebrew and retry.'
fi

if ! go_ready; then
  ensure_brew
  printf 'Installing Go...\n'
  brew install go
  export PATH="$(brew --prefix go)/bin:$PATH"
  hash -r
  go_ready || die 'Go 1.25.3 or newer is required. Update Go with Homebrew and retry.'
fi

printf 'Installing project dependencies and building the editor...\n'
npm ci --no-audit --no-fund
npm run build
mkdir -p .local
(
  cd backend
  go mod download
  go build -o "$project_dir/.local/brownnote" .
)

url="http://127.0.0.1:$port"
"$project_dir/.local/brownnote" -addr "127.0.0.1:$port" -static "$project_dir/dist" &
server_pid=$!
ready=false
for ((attempt=0; attempt<60; attempt++)); do
  kill -0 "$server_pid" 2>/dev/null || die 'The server exited before startup completed. Check the error above.'
  if curl --fail --silent --max-time 1 "$url/" --output /dev/null; then ready=true; break; fi
  sleep 1
done
[[ "$ready" == true ]] || die 'The server did not become ready within the startup window.'
printf '\nBrownnote is running at %s\nKeep this terminal open. Press Ctrl+C to stop.\n' "$url"
if ! open "$url"; then printf 'Open the address above in your browser.\n'; fi
wait "$server_pid"
