# Clowder 开发容器
#
# 宿主机是 Ubuntu 20.04，没有 WebKitGTK 4.1，所以 GUI 相关构建放到这个容器里。
# Go 工具链从宿主机挂载（/opt/go），容器里只装系统依赖。
#
# 构建：
#   docker build -t clowder-dev -f dev/Dockerfile .
# 使用：
#   dev/run.sh

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# 直接用清华源覆盖（deb822 格式），避免官方源在国内过慢导致索引拉不全
RUN printf '%s\n' \
        'Types: deb' \
        'URIs: http://mirrors.tuna.tsinghua.edu.cn/ubuntu/' \
        'Suites: noble noble-updates noble-backports noble-security' \
        'Components: main restricted universe multiverse' \
        'Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg' \
        > /etc/apt/sources.list.d/ubuntu.sources \
    && rm -f /etc/apt/sources.list

# MyGo 需要：GTK 3（原生 UI 和运行）、WebKitGTK 4.1（web 前端模式）
# 另装 xvfb 用于无头环境下验证窗口能否创建
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        ca-certificates \
        git \
        curl \
        libgtk-3-dev \
        libwebkit2gtk-4.1-dev \
        xvfb \
        x11-utils \
        imagemagick \
        fonts-noto-cjk \
        fontconfig \
    && rm -rf /var/lib/apt/lists/*

ENV GOROOT=/opt/go
ENV PATH=/opt/go/bin:/root/go/bin:$PATH
ENV GOPATH=/root/go
ENV GOCACHE=/root/.cache/go-build
ENV GOMODCACHE=/root/go/pkg/mod
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src
CMD ["bash"]
