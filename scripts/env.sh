# Clowder 开发环境
# 用法：source scripts/env.sh
#
# 本机 Go 工具链不在 PATH 里，放在 ~/go-toolchain 下，这里统一指过去。
# 缓存也复用该目录下的 gocache / gomodcache，避免重复下载。

export GOROOT="$HOME/go-toolchain/go"
export PATH="$GOROOT/bin:$PATH"

export GOPATH="$HOME/go-toolchain/gopath"
export GOCACHE="$HOME/go-toolchain/gocache"
export GOMODCACHE="$HOME/go-toolchain/gomodcache"

# 国内拉依赖走代理
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

go version
