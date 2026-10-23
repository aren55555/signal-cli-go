#!/usr/bin/env bash
# Sandbox preflight check. Run at the start of EVERY agent session, before
# anything else. Exits non-zero (and the session must stop) if any check
# fails. Also installed in the image as /usr/local/bin/sandbox-preflight.
set -u
fail=0
ok()   { printf '  ok    %s\n' "$1"; }
bad()  { printf '  FAIL  %s\n' "$1"; fail=1; }

echo "signal-cli-go sandbox preflight"

if [ "$(uname -s)" = "Linux" ]; then ok "OS is Linux"; else bad "OS is $(uname -s), not Linux"; fi

if [ -f /etc/signal-cli-go-sandbox ]; then
  ok "marker /etc/signal-cli-go-sandbox present ($(sed -n '2,6p' /etc/signal-cli-go-sandbox | tr '\n' ' '))"
else
  bad "marker /etc/signal-cli-go-sandbox missing"
fi

for p in /Users /Volumes /System/Library /private/var/db /Applications; do
  if [ -e "$p" ]; then bad "host path $p is visible"; fi
done
[ -e /Users ] || ok "no host paths visible"

if [ "${SIGNAL_CLI_GO_SANDBOX:-}" = "1" ]; then ok "SIGNAL_CLI_GO_SANDBOX=1"; else bad "SIGNAL_CLI_GO_SANDBOX is not set"; fi

if [ -d /work/.git ]; then ok "repo mounted at /work"; else bad "/work is not a git checkout"; fi

if [ "$(id -u)" != "0" ]; then ok "running as non-root uid $(id -u)"; else bad "running as root"; fi

if command -v go >/dev/null 2>&1; then ok "$(go version)"; else bad "go toolchain missing"; fi
if command -v java >/dev/null 2>&1; then ok "java $(java -version 2>&1 | head -n1)"; else bad "java missing"; fi
if command -v just >/dev/null 2>&1; then ok "$(just --version)"; else bad "just missing"; fi

# The only route out must be the allowlisting proxy.
if [ -n "${HTTPS_PROXY:-}" ]; then
  ok "HTTPS_PROXY=${HTTPS_PROXY}"
  if curl -sS --max-time 10 -o /dev/null -w '%{http_code}' https://api.anthropic.com/ >/dev/null 2>&1; then
    ok "proxy reachable (allowlisted host answered)"
  else
    bad "proxy did not relay to an allowlisted host (is the proxy service running?)"
  fi
  if code="$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' https://example.com/ 2>/dev/null)" && [ "$code" = "403" ]; then
    ok "non-allowlisted host refused by proxy (403)"
  elif [ "${code:-}" = "000" ] || [ -z "${code:-}" ]; then
    ok "non-allowlisted host unreachable"
  else
    bad "non-allowlisted host example.com returned HTTP ${code}; egress is not restricted"
  fi
else
  bad "HTTPS_PROXY is not set; egress is not going through the allowlisting proxy"
fi

if [ "$fail" -ne 0 ]; then
  echo "PREFLIGHT FAILED: not inside the sandbox, or the sandbox is misconfigured. Stop and tell the operator."
  exit 1
fi
echo "PREFLIGHT OK"
