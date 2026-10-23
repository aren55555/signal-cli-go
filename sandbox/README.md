# signal-cli-go sandbox

All work on this project happens inside this sandbox. The agent session
itself (Claude Code) runs in here, so builds, tests, the reference
`signal-cli` oracle, the JDK and the daemon never touch the host.

```
host (macOS)                      container runtime (Colima / Docker Desktop VM)
──────────────                    ──────────────────────────────────────────────
repo checkout  ── bind mount ──►  agent   /work          (internal network only)
                                    │ HTTPS_PROXY
                                    ▼
                                  proxy   :3128           (internal + egress network)
                                    │ only hosts in sandbox/ALLOWED_HOSTS
                                    ▼
                                  Internet
```

## What is in the image

Everything is pinned by version **and** SHA-256 in `Containerfile`:

| Component   | Version                    | Why                                          |
|-------------|----------------------------|----------------------------------------------|
| Debian      | bookworm-slim @sha256:7ff6…| base image, pinned by index digest           |
| Go          | 1.27.3                     | the only toolchain the project code needs    |
| Temurin JDK | 25 (25.0.5+8 on amd64)     | signal-cli 0.14.7 requires Java 25           |
| just        | 1.59.0                     | task runner (`just` lists recipes)           |
| Claude Code | 2.1.309                    | the agent session, auto-update disabled      |

The reference `signal-cli` 0.14.7 is **not** baked in. Milestone 0 adds a
repo script that downloads it into the gitignored `third_party/` directory,
verifies its SHA-256 and GPG signature, and runs it only inside the sandbox.

The image bakes in the marker file `/etc/signal-cli-go-sandbox` and sets
`SIGNAL_CLI_GO_SANDBOX=1`. `preflight.sh` checks both, plus that the OS is
Linux, no host paths are visible, the repo is at `/work`, and that egress
really goes through the allowlisting proxy.

## Architecture: why linux/amd64

signal-cli 0.14.7 ships its libsignal native library for x86_64 Linux only.
This repo's host is an Apple Silicon Mac, so by default the image is built
for `linux/amd64` and run under Rosetta. That keeps the oracle unmodified,
which matters: it is the thing we test against.

If you prefer a native arm64 image, set `SANDBOX_PLATFORM=linux/arm64`
(the Containerfile carries arm64 pins too). The oracle will then need an
aarch64 `libsignal_jni` library from a source the operator trusts; the
project does not supply one. Go builds and tests would be faster, the
oracle would be a bigger trust question. amd64 is the recommended default.

## Prerequisites on the host

A container runtime that can run amd64 Linux containers. Nothing else.
Either:

- **Colima** (already installed via mise on this host). Start it with the
  `vz` VM type and Rosetta so amd64 containers run at near-native speed:

  ```sh
  colima start --vm-type vz --vz-rosetta --cpu 4 --memory 8 --disk 60
  ```

  If Colima is already running with a smaller profile (the defaults are
  2 CPUs and 2 GiB, too little for a JVM plus Go test runs plus Claude
  Code), `colima stop` first; `start` flags are ignored on a running VM.
  Rosetta must be installed on the host (`softwareupdate --install-rosetta`)
  and enabled in the profile (`rosetta: true` in
  `~/.colima/default/colima.yaml`); without it amd64 containers still run,
  but under slow QEMU emulation.

  Colima mounts your home directory into the VM by default, which is what
  lets the repo bind-mount work. Nothing in this repo mounts any other path.

- **Docker Desktop** with "Use Rosetta for x86_64/amd64 emulation" enabled
  in Settings → General.

## Build

The build needs BuildKit (the `buildx` CLI plugin): it sets `TARGETARCH`
and honors the `platform` field, which the classic builder does not. It
also needs the `compose` plugin. The mise `docker-cli` package ships
neither; install both through mise and link them where the Docker CLI
looks for plugins:

```sh
mise use -g aqua:docker/buildx@0.38.1 docker-compose@5.6.1
mkdir -p ~/.docker/cli-plugins
ln -sf "$(mise where aqua:docker/buildx)/docker-cli-plugin-docker-buildx" ~/.docker/cli-plugins/docker-buildx
ln -sf "$(mise which docker-compose)" ~/.docker/cli-plugins/docker-compose
docker buildx version && docker compose version
```

From the repo root:

```sh
docker compose -f sandbox/compose.yaml build
```

The build context is `sandbox/` only (the repo is never sent to the
daemon). It downloads Go, the JDK, `just` and Claude Code and fails if any
checksum does not match the pins in `Containerfile`. Expect a few minutes
and roughly 1.5 GB.

If your host uid is not 1000 and you are on a Linux host, build with
`AGENT_UID=$(id -u) AGENT_GID=$(id -g)` in the environment so files created
in `/work` are owned by you. On macOS the runtime maps ownership for you.

## Launch

```sh
docker compose -f sandbox/compose.yaml run --rm agent
```

This starts the `proxy` service (if not already running) and drops you into
a shell as the non-root `agent` user in `/work`. Then, inside:

```sh
sandbox/preflight.sh      # must print PREFLIGHT OK
claude                    # start the agent session
```

The first `claude` run asks you to log in. It prints a URL; open it in a
browser on the host and paste the code back. The login state is stored in
the `agent-home` named volume, which lives inside the container runtime,
never on the host. Alternatively export `ANTHROPIC_API_KEY` in the host
shell before `run`; compose passes it through.

To stop everything, including the proxy:

```sh
docker compose -f sandbox/compose.yaml down
```

To throw away the sandbox's internal state (login, Go cache) as well:

```sh
docker compose -f sandbox/compose.yaml down --volumes
```

## Git identity

The sandbox has no git identity of its own. Set yours once, from the host,
in the repo-local config, which lives in the mounted `.git` directory:

```sh
git config user.name "Your Name"
git config user.email "you@example.com"
```

Commits made inside the sandbox then carry that identity. The sandbox has
no credentials, so pushing is done from the host.

## Secrets

The operator passes staging test numbers and captcha tokens in as
environment variables when launching:

```sh
SIGNAL_STAGING_NUMBER_A=+1... SIGNAL_STAGING_NUMBER_B=+1... \
SIGNAL_CAPTCHA_TOKEN=signalcaptcha://... \
docker compose -f sandbox/compose.yaml run --rm agent
```

They exist only in the container's environment. They are never written to
the host and never committed; a repo test scans `testdata/` for leaked
identifiers.

## Network egress

The `agent` container is attached only to the `agentnet` network, which is
declared `internal: true`: it has no gateway to the outside. The `proxy`
container is on both `agentnet` and the external `egress` network, and
forwards only to hosts listed in `ALLOWED_HOSTS` (CONNECT tunnels to ports
443/80 and plain HTTP). Denied requests get a 403 and appear in
`docker compose -f sandbox/compose.yaml logs proxy`.

Inside the sandbox the proxy is configured through `HTTPS_PROXY` (honored
by Claude Code, curl, and Go's `net/http`) and through `JAVA_OPTS` for the
JVM oracle. `signal-cli-go`'s own WebSocket client must therefore support
CONNECT proxies from day one, which is a requirement in PLAN.md.

Go cannot download anything: `GOPROXY=off` and `GOTOOLCHAIN=local` are set
in the image, so a stray `go get` fails rather than quietly adding a
dependency.

**Adding a host to `ALLOWED_HOSTS` requires the operator's approval.** The
proxy reads the file at start, so after editing it run
`docker compose -f sandbox/compose.yaml restart proxy`.

Known limitation: DNS lookups from the agent container are still answered
by the runtime's embedded resolver, so hostname resolution itself is not
filtered. Only connections are.

## Hardening summary

- non-root user, `cap_drop: ALL`, `no-new-privileges`, pids limit, `/tmp` on tmpfs
- no privileged mode, no added capabilities, no device access
- no runtime socket, no `$HOME`, `~/.ssh`, `~/.config` or keychain mounted
- only the repo checkout is bind-mounted; everything else lives in a named volume
- the sandbox has no git credentials: commit inside, push from the host
- the proxy runs from an empty (`scratch`) image, read-only, as nobody

## Re-pinning

Pins live at the top of `Containerfile` with the URL each one came from.
To bump one, fetch the new checksum from that source, update the `ARG`,
rebuild. Base-image digests come from the Docker Hub tag API; the JDK
checksums from the Adoptium API; Go from `go.dev/dl/?mode=json`; `just`
from its release `SHA256SUMS`; Claude Code from its release `manifest.json`.
Debian packages installed with `apt-get` in the base stage are not pinned
beyond the base image digest; pin them to a `snapshot.debian.org` date if
you need bit-for-bit reproducibility.
