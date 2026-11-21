# Building libghostty-vt for Clowder

Clowder's terminal pane uses [libghostty-vt](https://github.com/ghostty-org/ghostty),
Ghostty's terminal emulation core, through the Go bindings
[`go.mitchellh.com/libghostty`](https://github.com/mitchellh/go-libghostty).
The library is a C ABI with no dependencies beyond libc, so it links statically.

## Requirements

- Ghostty's source requires **Zig 0.17.1** (see `build.zig.zon`, `minimum_zig_version`)
- Go 1.27+

## Build

```sh
# 1. Zig 0.17.1
curl -LO https://ziglang.org/download/0.17.1/zig-x86_64-linux-0.17.1.tar.xz
tar -xf zig-x86_64-linux-0.17.1.tar.xz

# 2. Ghostty source
git clone --depth 1 https://github.com/ghostty-org/ghostty.git

# 3. Build the library only
cd ghostty
../zig-x86_64-linux-0.17.1/zig build -Demit-lib-vt \
    --prefix ../prefix -Doptimize=ReleaseFast

# 4. Point the Go build at it
export PKG_CONFIG_PATH=$PWD/../prefix/lib/pkgconfig
```

Takes about four minutes on a modern machine and produces:

```
prefix/lib/libghostty-vt.a
prefix/lib/pkgconfig/libghostty-vt.pc
prefix/lib/pkgconfig/libghostty-vt-static.pc
prefix/include/...
```

Then `CGO_ENABLED=1 go build ./...` links it statically.

## Notes

- The Go binding's `pkg-config` file records the `--prefix` used at build time.
  Mount or install the library at that same absolute path, or the link step
  fails looking for `libghostty-vt.a`.
- Ghostty's repo needs Zig 0.17.x; older Zig (0.15) fails at configure time.
- On a network where `git clone` from GitHub stalls, clone on another host and
  copy the tree in; the build itself needs no network once
  `zig build` has fetched its lazy dependencies once.
