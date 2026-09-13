# TLS path

Vakt requires a secure context: iOS registers a service worker, and delivers
Web Push, only to a page served over HTTPS with a certificate the device
trusts. See SDD.md §2.4 for why this gates the whole MVP.

**Chosen path: Tailscale, via `tailscale serve`.** Not raw `tailscale cert`
file management — `tailscale serve` terminates TLS itself, using a
certificate tailscaled issues, installs, and renews automatically. Nothing
in this repo generates, stores, or rotates a certificate.

## Prerequisites

One-time, per tailnet, in the Tailscale admin console:

- **MagicDNS** enabled (`https://login.tailscale.com/admin/dns`) — gives
  every node a stable `*.ts.net` name.
- **HTTPS Certificates** enabled on the same page — without this,
  `tailscale cert` and `tailscale serve` both fail with
  `your Tailscale account does not support getting TLS certs`.

Both are tailnet-wide settings, not per-device.

## Standing it up

```
tailscale serve --bg <port>
```

exposes `http://127.0.0.1:<port>` at `https://<magicdns-name>/` to every
device on the tailnet — including a phone that has the Tailscale app
installed and is signed into the same tailnet, with no port, cert file, or
`Host` header handling for the app to own. `tailscale serve status` shows
the current mapping; `tailscale serve --https=443 off` tears it down.

For local development, point it at `vaktd`'s port once it's running
(`make up` binds `:8080` by default):

```
tailscale serve --bg 8080
```

## Where this lands for later tasks

- `build-pwa-shell` needs a real HTTPS origin to register the actual
  manifest and service worker — point `tailscale serve` at the dev server
  the same way.
- `create-docker-compose` / M6 packaging decide how this runs in the
  container (`tailscaled` + `tailscale serve` inside the container vs. as a
  sidecar). Not decided here — this document fixes the TLS *mechanism*,
  not the deployment topology.

## Fallbacks (not needed — kept for the record)

SDD.md §2.4 names `mkcert` (local CA, installed per device) and DNS-01 ACME
with split-horizon DNS as alternatives if the Tailscale path proved
unworkable. It didn't — see Verification below — so neither is built.

## Verification

Confirmed on a real device (iPhone, Safari, same tailnet):

1. A minimal page + service worker was served locally and exposed via
   `tailscale serve` at `https://<node>.<tailnet>.ts.net/`.
2. Safari loaded it over HTTPS with **no certificate warning**.
3. `navigator.serviceWorker.register()` resolved — the secure-context
   requirement is satisfied end to end.

The verification page was throwaway (not committed) — `build-pwa-shell`
builds the real manifest and service worker against this same TLS path.
