# Clowder development environment
# Usage: source scripts/env.sh
#
# This machine keeps its Go toolchain outside PATH, under ~/go-toolchain.
# Caches live there too, so dependencies are not downloaded twice.

export GOROOT="$HOME/go-toolchain/go"
export PATH="$GOROOT/bin:$PATH"

export GOPATH="$HOME/go-toolchain/gopath"
export GOCACHE="$HOME/go-toolchain/gocache"
export GOMODCACHE="$HOME/go-toolchain/gomodcache"

# Use a proxy for module downloads
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

go version
