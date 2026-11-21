#!/usr/bin/env bash
# 在开发容器里执行命令。
#
#   dev/run.sh go build ./...
#   dev/run.sh bash            # 进交互 shell
#   dev/run.sh gui             # 无头启动 GUI 并截图（验证窗口能创建）
#
# Go 工具链、缓存都从宿主机挂载，容器本身无状态。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${CLOWDER_DEV_IMAGE:-clowder-dev}"
GO_TC="$HOME/go-toolchain"

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "镜像 $IMAGE 不存在，先构建：" >&2
  echo "  docker build -t $IMAGE -f dev/Dockerfile ." >&2
  exit 1
fi

# gui 子命令：在 Xvfb 里跑程序，然后截图
if [ "${1:-}" = "gui" ]; then
  shift
  TARGET="${1:-./bin/clowder-ui}"
  docker run --rm \
    --user "$(id -u):$(id -g)" \
    -e HOME=/tmp \
    -v "$GO_TC/go:/opt/go:ro" \
    -v "$GO_TC/gocache:/gocache" \
    -v "$GO_TC/gomodcache:/gomodcache" \
    -v "$ROOT:/src" \
    -w /src \
    -e GOCACHE=/gocache \
    -e GOMODCACHE=/gomodcache \
    -e GOPATH=/tmp/gopath \
    -e GOPROXY=https://goproxy.cn,direct \
    -e CLOWDER_GUI_WAIT \
    -e CLOWDER_AUTOSTART \
    -e CLOWDER_MOCKUP \
    -e CLOWDER_TAB \
    -e CLOWDER_PAGE \
    -e CLOWDER_DEMO \
    -e LANG \
    -e LC_ALL \
    "$IMAGE" bash -c "
      Xvfb :99 -screen 0 1280x800x24 >/tmp/xvfb.log 2>&1 &
      sleep 2
      DISPLAY=:99 $TARGET >/tmp/app.log 2>&1 &
      APP=\$!
      sleep "${CLOWDER_GUI_WAIT:-8}"
      DISPLAY=:99 import -window root /src/.gui.png 2>/dev/null || true
      kill \$APP 2>/dev/null || true
      echo '--- app.log ---'; tail -30 /tmp/app.log
    "
  exit 0
fi

exec docker run --rm -i \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -v "$GO_TC/go:/opt/go:ro" \
  -v "$GO_TC/gocache:/gocache" \
  -v "$GO_TC/gomodcache:/gomodcache" \
  -v "$ROOT:/src" \
  -w /src \
  -e GOCACHE=/gocache \
  -e GOMODCACHE=/gomodcache \
  -e GOPATH=/tmp/gopath \
  -e GOPROXY=https://goproxy.cn,direct \
  -e CGO_ENABLED=1 \
  ${DOCKER_TTY:+-t} \
  "$IMAGE" "${@:-bash}"
