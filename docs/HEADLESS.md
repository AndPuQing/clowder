# Clowder headless on Linux

`clowderd` is the same tunnel engine as the desktop app without GTK,
WebKitGTK or a GUI. The release archive contains one statically linked
binary: it runs on a minimal image with no libc or desktop packages.

## Install

```sh
tar -xzf clowderd-linux-amd64.tar.gz
sudo install -m 0755 clowderd /usr/local/bin/
clowderd --version
```

Use `clowderd-linux-arm64.tar.gz` on ARM64 servers.

## Use

```sh
# share this machine's 8080
clowderd serve -expose 8080

# also relay UDP, and let the peer reach addresses this machine can reach
clowderd serve -expose 8080 -udp -allow-proxy

# keep the address across restarts
clowderd serve -expose 8080 -key clowderd

# connect to someone else's host
clowderd dial -addr tc... -forward 9000:8080 -socks 127.0.0.1:1080

# list or fetch a file from a host that shares a directory
clowderd files -addr tc... -list /
clowderd files -addr tc... -get /notes.md -out ./notes.md
```

The address is written to stdout. With `-key`, the same address is
reported again after a restart; without it, a new address is generated
for every run.

## systemd

The archive includes `clowderd.service`. It reads its arguments from
`/etc/clowderd/clowderd.env`:

```sh
sudo install -m 0755 clowderd /usr/local/bin/
sudo install -m 0644 clowderd.service /etc/systemd/system/
sudo install -d -m 0755 /etc/clowderd
printf 'CLOWDERD_ARGS=-expose 8080 -key clowderd\n' \
  | sudo tee /etc/clowderd/clowderd.env

sudo systemctl daemon-reload
sudo systemctl enable --now clowderd
journalctl -u clowderd -f
```

The unit runs as a dynamic user with its state under `/var/lib/clowder`
(`StateDirectory`); the key and settings survive restarts there.

For a public-facing service, enable the host's client allow-list and
give the peer the client key to paste:

```sh
clowderd serve -expose 8080 -key clowderd -allow nodekey:...
```

## Notes

- The binary is static (`CGO_ENABLED=0`); `file clowderd` reports
  "statically linked" and `ldd` reports "not a dynamic executable".
- No port has to be opened in the firewall: the tunnel is outbound to a
  DERP relay and becomes peer-to-peer when a direct path exists.
- `-allow-proxy` is powerful: it lets the holder of the address reach any
  address this machine can dial. Leave it off unless that is the point.
- The desktop app and the headless binary share `~/.config/clowder`
  (devices, settings and keys), so `-key` and the GUI use the same key
  store on the same machine.
