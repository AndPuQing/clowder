# Build and check orchestration for Clowder (just 1.0+).
#
#   just                 # build clowder and clowderd into bin/
#   just --list          # list every recipe
#   just doctor          # check the local toolchain when a build fails
#
# The Go toolchain is not on PATH (see scripts/env.sh); every recipe points
# at it. Override with CLOWDER_TOOLCHAIN.

set shell := ["bash", "-uc"]

source_dir := canonicalize(justfile_dir())
toolchain  := env_var_or_default("CLOWDER_TOOLCHAIN", home_directory() / "go-toolchain")
goroot     := toolchain / "go"

ghostty_prefix := env_var_or_default("CLOWDER_GHOSTTY_PREFIX", source_dir / ".ghostty-build/prefix")
ghostty_src    := env_var_or_default("CLOWDER_GHOSTTY_SRC", source_dir / ".ghostty-build/ghostty")
zig            := env_var_or_default("CLOWDER_ZIG", source_dir / ".ghostty-build/zig-x86_64-linux-0.17.1/zig")
zig_cache      := env_var_or_default("CLOWDER_ZIG_CACHE", ghostty_src / ".zig-cache")
pc_dir         := source_dir / ".ghostty-build/pkgconfig"

raw_version := trim(`git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev`)
version     := env_var_or_default("VERSION", raw_version)
ldflags     := "-X github.com/AndPuQing/clowder/core.Version=" + version

export GOROOT     := goroot
export GOPATH     := toolchain / "gopath"
export GOCACHE    := env_var_or_default("GOCACHE", toolchain / "gocache")
export GOMODCACHE := toolchain / "gomodcache"
export GOPROXY    := env_var_or_default("GOPROXY", "https://goproxy.cn,direct")
export PATH       := (goroot / "bin") + ":" + env_var_or_default("PATH", "/usr/local/bin:/usr/bin:/bin")

# Default recipe: build both binaries.
default: build

alias b := build
alias t := test
alias c := check

# Build the desktop app and the command-line tool into bin/.
build: app cli

# Build only the desktop app (GTK3 / WebKitGTK at runtime).
app: _bin
    go build -trimpath -ldflags "{{ldflags}}" -o bin/clowder ./cmd/clowder

# Build only the command-line tool (no GUI dependency; good for servers).
# Static: CGO off means it runs on a minimal Linux image.
cli: _bin
    CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}}" -o bin/clowderd ./cmd/clowderd

# Build a stripped release build with the version pinned to this commit.
build-release: _bin
    go build -trimpath -ldflags "{{ldflags}} -s -w" -o bin/clowder ./cmd/clowder
    CGO_ENABLED=0 go build -trimpath -ldflags "{{ldflags}} -s -w" -o bin/clowderd ./cmd/clowderd

# Build the headless CLI as a static Linux binary for servers. No GTK,
# WebKitGTK or libc: it runs on a minimal image.
headless arch="amd64": _bin
    CGO_ENABLED=0 GOOS=linux GOARCH={{arch}} \
        go build -trimpath -ldflags "{{ldflags}} -s -w" \
        -o bin/clowderd-linux-{{arch}} ./cmd/clowderd

# Package the headless CLI with a systemd unit, install notes and checksum.
package-headless arch="amd64": (headless arch)
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    d="dist/headless-{{arch}}"
    rm -rf "$d"
    mkdir -p "$d"
    cp "bin/clowderd-linux-{{arch}}" "$d/clowderd"
    cp packaging/clowderd.service "$d/"
    cp docs/HEADLESS.md "$d/README.md"
    cp LICENSE "$d/"
    tar -czf "dist/clowderd-linux-{{arch}}.tar.gz" -C "$d" .
    if command -v sha256sum >/dev/null 2>&1; then
        ( cd dist && sha256sum "clowderd-linux-{{arch}}.tar.gz" \
            > "clowderd-linux-{{arch}}.sha256" )
    else
        ( cd dist && shasum -a 256 "clowderd-linux-{{arch}}.tar.gz" \
            > "clowderd-linux-{{arch}}.sha256" )
    fi
    echo "wrote dist/clowderd-linux-{{arch}}.tar.gz"

# Build the desktop app with the interactive terminal (libghostty).
# Run `just ghostty-lib` first.
build-ghostty: _ghostty-pc _bin
    CGO_ENABLED=1 PKG_CONFIG_PATH="{{pc_dir}}" \
        go build -tags libghostty -trimpath -ldflags "{{ldflags}}" -o bin/clowder ./cmd/clowder

# Build libghostty-vt from source with Zig into .ghostty-build/prefix.
# Takes a few minutes and only has to run once.
ghostty-lib:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! -d "{{ghostty_src}}" ]]; then
        echo "Ghostty source missing: {{ghostty_src}}" >&2
        echo "Clone it as described in docs/GHOSTTY.md, or set CLOWDER_GHOSTTY_SRC" >&2
        exit 1
    fi
    if [[ ! -x "{{zig}}" ]]; then
        echo "Zig 0.17.1 missing: {{zig}}" >&2
        echo "See docs/GHOSTTY.md, or set CLOWDER_ZIG" >&2
        exit 1
    fi
    for d in "{{ghostty_prefix}}" "{{zig_cache}}"; do
        if [[ -e "$d" && ! -w "$d" ]]; then
            echo "Not writable: $d" >&2
            echo "It was probably created as root inside the container. Fix it with:" >&2
            echo "  sudo chown -R \"$(id -un)\" \"{{source_dir}}/.ghostty-build\"" >&2
            echo "or point CLOWDER_GHOSTTY_PREFIX / CLOWDER_ZIG_CACHE somewhere writable." >&2
            exit 1
        fi
    done
    mkdir -p "{{zig_cache}}" "{{ghostty_prefix}}"
    cd "{{ghostty_src}}"
    "{{zig}}" build -Demit-lib-vt --prefix "{{ghostty_prefix}}" \
        --cache-dir "{{zig_cache}}" -Doptimize=ReleaseFast
    echo "Built {{ghostty_prefix}}/lib/libghostty-vt.a"

# Run the unit tests.
test:
    go test ./...

# Run the tests with the race detector (the desktop side needs it most).
test-race:
    go test -race ./...

# Run go vet.
vet:
    go vet ./...

# Check formatting (used by CI).
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    out="$(gofmt -l cmd core ui)"
    if [[ -n "$out" ]]; then
        echo "These files need gofmt:" >&2
        echo "$out" >&2
        exit 1
    fi

# Format the sources.
fmt:
    gofmt -w cmd core ui

# Pre-commit check: formatting, vet, tests.
check: fmt-check vet test

# Full local verification: check, build, smoke test.
ci: check build smoke

# Smoke test: the binary starts and prints its version.
smoke: build
    ./bin/clowder --version

# End-to-end check: tunnel a local HTTP file through DERP (needs network).
e2e: cli
    scripts/e2e.sh

# Run the desktop app.
run: app
    ./bin/clowder

# Run the desktop app with the interactive terminal.
run-ghostty: build-ghostty
    ./bin/clowder

# Render the interface direction drafts (A/B/C; needs a display).
mockups dir="A":
    CLOWDER_MOCKUP={{dir}} go run ./cmd/mockups

# Open a shell in the development container (run `just image` first).
dev:
    DOCKER_TTY=1 dev/run.sh bash

# Run one command in the development container: just dev-run go build ./...
dev-run *cmd:
    dev/run.sh {{cmd}}

# Build the development image (all Linux GUI dependencies live there).
image:
    docker build -t "${CLOWDER_DEV_IMAGE:-clowder-dev}" -f dev/Dockerfile .

# Start the GUI headlessly and screenshot it to .gui.png.
# Works with CLOWDER_PAGE / CLOWDER_DEMO / CLOWDER_TAB in the environment.
shot target="bin/clowder": app
    dev/run.sh gui {{target}}

# Check whether this machine can build: toolchain, GUI runtime, libghostty.
doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    ok()  { printf '  ok   %s\n' "$1"; }
    bad() { printf '  FAIL %s\n' "$1"; }

    echo "Go"
    if command -v go >/dev/null 2>&1; then
        ok "$(go version)"
    else
        bad "go not found; expected {{goroot}}/bin/go (set CLOWDER_TOOLCHAIN)"
    fi

    echo "Desktop runtime libraries (Linux; building needs no -dev packages)"
    if [[ "$(uname -s)" == "Linux" ]]; then
        if grep -q 'libgtk-3\.so' < <(ldconfig -p 2>/dev/null); then
            ok "libgtk-3"
        else
            bad "libgtk-3-0 missing (required to run the app)"
        fi
        if grep -qE 'libwebkit2gtk-4\.[01]\.so' < <(ldconfig -p 2>/dev/null); then
            ok "libwebkit2gtk"
        else
            bad "libwebkit2gtk missing (needed for the web view; optional for native UI)"
        fi
        if grep -qE 'libayatana-appindicator3|libappindicator3' < <(ldconfig -p 2>/dev/null); then
            ok "tray library"
        else
            bad "tray library missing (without it, closing the window quits)"
        fi
    else
        ok "not Linux; skipping GTK checks"
    fi

    echo "libghostty (interactive terminal)"
    if [[ -f "{{ghostty_prefix}}/lib/libghostty-vt.a" ]]; then
        ok "{{ghostty_prefix}}/lib/libghostty-vt.a"
    else
        bad "libghostty-vt.a missing; run just ghostty-lib"
    fi
    if [[ -x "{{zig}}" ]]; then
        ok "zig 0.17.1"
    else
        bad "Zig missing; see docs/GHOSTTY.md"
    fi

    echo "Other"
    command -v just >/dev/null 2>&1 && ok "just $(just --version | awk '{print $2}')" || bad "just not installed"
    command -v docker >/dev/null 2>&1 && ok "docker" || bad "docker not installed (only needed for dev/shot)"

# Remove build artifacts (leaves the local libghostty build alone).
clean:
    rm -rf bin dist

# Create bin/ (internal).
_bin:
    mkdir -p bin

# Generate pkg-config files: the library's .pc records the prefix used at
# build time, which often differs from where it actually lives, so rewrite
# it to the local path.
#
# The static library calls arc4random_buf, which glibc only provides from
# 2.36 on. On older glibc we link libbsd, which exports the same symbol, and
# record that in Libs.private so plain `pkg-config --static` picks it up.
_ghostty-pc:
    #!/usr/bin/env bash
    set -euo pipefail
    src="{{ghostty_prefix}}/lib/pkgconfig"
    dst="{{pc_dir}}"
    if [[ ! -f "$src/libghostty-vt.pc" ]]; then
        echo "$src/libghostty-vt.pc missing; run just ghostty-lib first" >&2
        exit 1
    fi
    mkdir -p "$dst"
    for f in "$src"/*.pc; do
        sed -E "s#^prefix=.*#prefix={{ghostty_prefix}}#" "$f" > "$dst/$(basename "$f")"
    done

    private=""
    if [[ "$(uname -s)" == "Linux" ]]; then
        # Note: command substitutions run under pipefail, so end them with
        # `|| true`; awk's `exit` makes the upstream command die of SIGPIPE.
        libc="$(ldconfig -p 2>/dev/null | awk '/libc\.so\.6 /{print $NF; exit}' || true)"
        libc_syms=""
        if [[ -n "$libc" ]]; then
            libc_syms="$(nm -D "$libc" 2>/dev/null || true)"
        fi
        if [[ "$libc_syms" != *" arc4random_buf"* ]]; then
            bsd="$(ldconfig -p 2>/dev/null | awk '/libbsd\.so\.0 /{print $NF; exit}' || true)"
            if [[ -z "$bsd" ]]; then
                echo "glibc lacks arc4random_buf and libbsd is missing; install libbsd" >&2
                exit 1
            fi
            # There is usually no libbsd.so development symlink, so make one.
            mkdir -p "$dst/lib"
            ln -sfn "$bsd" "$dst/lib/libbsd.so"
            private="-L$dst/lib -lbsd"
        fi
    fi
    if [[ -n "$private" ]]; then
        pc="$dst/libghostty-vt-static.pc"
        sed -E "s#^Libs.private:.*#Libs.private: $private#" "$pc" > "$pc.tmp"
        mv "$pc.tmp" "$pc"
    fi
