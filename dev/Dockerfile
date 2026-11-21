# Clowder development container
#
# The host is Ubuntu 20.04 without WebKitGTK 4.1, so GUI builds happen in
# this container. The Go toolchain is mounted from the host (/opt/go); the
# container only provides system dependencies.
#
# Build:
#   docker build -t clowder-dev -f dev/Dockerfile .
# Use:
#   dev/run.sh

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Use the Tsinghua mirror (deb822 format): the official mirrors are slow
# from mainland China and sometimes truncate the index
RUN printf '%s\n' \
        'Types: deb' \
        'URIs: http://mirrors.tuna.tsinghua.edu.cn/ubuntu/' \
        'Suites: noble noble-updates noble-backports noble-security' \
        'Components: main restricted universe multiverse' \
        'Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg' \
        > /etc/apt/sources.list.d/ubuntu.sources \
    && rm -f /etc/apt/sources.list

# MyGo needs GTK 3 (native UI and runtime) and WebKitGTK 4.1 (web frontend
# mode). xvfb verifies that a window can be created headlessly.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        pkg-config \
        ca-certificates \
        git \
        curl \
        libgtk-3-dev \
        libwebkit2gtk-4.1-dev \
        libayatana-appindicator3-dev \
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
