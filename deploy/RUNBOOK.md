# Operator runbook

This accumulates as decisions are made (SDD.md §7) rather than being
written from memory at the end. Sections 1–8 are the install-and-operate
story for someone who has never seen this repository. "For developers"
at the end covers build-time verification, not day-to-day operation.

## 1. Prerequisites

- **Docker and Docker Compose.** The image is built from this repo's
  `Dockerfile`; there is no published registry image, so obtaining the
  software means cloning (or otherwise copying) this repository onto the
  host that will run it.
- **Network reachability.** The container listens on one port
  (`VAKT_PORT`, default `8080`). Reaching it from a phone requires either
  the same LAN or a tunnel — see TLS setup below, which is also how a
  phone gets a trusted HTTPS origin.
- **An iOS device to receive pushes.** Web Push on iOS only works from a
  page added to the Home Screen, opened in that installed mode, served
  over HTTPS from a trusted certificate, in Safari. Neither the PRD nor
  the SDD pins an exact iOS version floor for Home Screen Web Push; treat
  a current iOS release as the baseline and verify on the actual device
  you intend to use.

## 2. TLS setup

iOS requires a secure context before it will register a service worker or
deliver a push, so plain HTTP to a LAN IP does not work. The chosen
mechanism is `tailscale serve`, which terminates TLS with a certificate
Tailscale issues and renews — nothing in this repo generates or stores a
certificate. In short: enable MagicDNS and HTTPS Certificates once in the
Tailscale admin console, then run `tailscale serve --bg <port>` pointed
at the port `vaktd` publishes. Full details, including the settings that
must be enabled first and how to tear it down: `deploy/TLS.md`.

## 3. Volume and environment contract

### Volumes

| Volume | Mount | Contents | Backup implication |
|---|---|---|---|
| Vault | `${VAULT_PATH:-./vault}` on the host -> `/vault` in the container | The operator's own Markdown task files | Already whatever the operator's own backup/git story for their Markdown is; Vakt does not add to it |
| Config | named volume `vakt-config` -> `/config` in the container | `vapid.json` (VAPID keypair), `subscriptions.json` (enrolled devices) | Vakt-owned state with no equivalent elsewhere; back it up separately (§6) |

### Environment variables

Set via `docker-compose.yml`'s `environment:` block or a `.env` file next
to it:

| Variable | Default | Operator-set? | Controls |
|---|---|---|---|
| `VAKT_PORT` | `8080` | Yes, if 8080 is taken on the host | Host-side port published to the container's `8080` |
| `VAULT_PATH` | `./vault` | Yes | Host directory bind-mounted as the vault |
| `TZ` | `UTC` | Yes | Alpine's own OS clock/log timestamps only |
| `VAKT_TZ` | unset (falls back to `time.Local` in the container) | Yes | The IANA timezone every `@schedule`/`@once` directive is evaluated in (SDD.md G9) |
| `VAKT_VAPID_CONTACT` | `mailto:admin@example.com` | **Yes — set this to a real, reachable address** | The RFC 8292 `sub` claim every VAPID JWT carries |

`TZ` and `VAKT_TZ` are independent — setting one does not set the other.
Leaving `VAKT_TZ` unset ties scheduling to the container's own local time,
which is usually not what's intended; set it explicitly (e.g.
`VAKT_TZ=Europe/Bucharest`).

**`VAKT_VAPID_CONTACT` is not cosmetic.** Confirmed against a real device
and Apple's production push endpoint during milestone acceptance: a push
service validates this claim against a real, resolvable domain. The
built-in fallback (`mailto:admin@example.com`, a real domain reserved for
documentation by RFC 2606) works, but a `localhost` or otherwise
non-resolvable contact does not — every push then fails with a 403
`BadJwtToken`, with an otherwise completely correct request (valid
signature, fresh subscription, everything else right). Set this to an
address you actually control before relying on notifications.

These are baked into the image by the `Dockerfile` and are not meant to
be operator-set — listed for completeness:

| Variable | Baked default | Why not operator-set |
|---|---|---|
| `VAKT_ADDR` | `:8080` | Internal bind address; `VAKT_PORT` is the operator-facing knob |
| `VAKT_WEB_DIR` | `/app/web/dist` | Fixed location of the built frontend inside the image |
| `VAKT_CONFIG_DIR` | `/config` | Must match the `/config` volume mount |
| `VAKT_VAULT_DIR` | set by `docker-compose.yml` to `/vault` | Must match the vault volume mount |

## 4. First run

1. Obtain the repo (`git clone` or copy) onto the host.
2. Optionally create a `.env` file next to `docker-compose.yml` setting
   `VAKT_PORT`, `VAULT_PATH`, `TZ`, `VAKT_TZ`.
3. `docker compose up -d`
4. Confirm it's healthy:
   - `docker compose logs -f vaktd` — expect a line like
     `vaktd listening on :8080, serving frontend from /app/web/dist`
     and no repeated errors.
   - `curl http://localhost:${VAKT_PORT:-8080}/healthz` — expect a `200`.
5. Reach the app over the TLS origin from §2 on the iOS device and open
   it in Safari. A vault with no tasks yet shows a "Getting started" card
   ("Add a task to a Markdown file in your vault — it shows up in the
   feed below once it's due.") with a way to enable notifications below
   it — see enrolment next.

## 5. Enrolment

What the device actually shows, from `first-run-guide.tsx` and
`push-enrolment-dialog.tsx`:

1. If the app is not yet running in standalone (Home Screen) mode, the
   "Getting started" card first says: "To be notified, add Vakt to your
   Home Screen first: open the share menu and choose 'Add to Home
   Screen', then open Vakt from that icon." Do that, then reopen the app
   from the new Home Screen icon.
2. Tap **Enable notifications**. A dialog explains what notifications are
   for ("the household know when something is due... Nothing else
   changes") and offers **Turn on notifications**.
3. Tapping it triggers the OS permission prompt (`Notification.request
   Permission()`, which iOS requires to originate from this tap). Accept
   it.
   - **Declined:** the dialog explains how to re-enable later from the
     device's own Settings app, under Vakt.
   - **Dismissed without a choice:** the dialog stays open; it does not
     re-prompt on its own.
   - **Accepted:** the client subscribes via the Push API, posts the
     subscription to the server, and the button becomes "Notifications
     on."
4. Once enrolled, the "Getting started" card is replaced by "Notifications
   are on for this device."

Repeat per device — enrolment is per-browser-instance, not per-household.

## 6. Backup and restore of `/config`

`/config` holds `vapid.json` (the VAPID keypair) and `subscriptions.json`
(one entry per enrolled device). Both are written with atomic temp-file
+ rename, so the daemon never leaves a torn file on disk — a live copy
taken while the container is running is safe; stopping the container
first is not required.

**Backup:** copy the whole `/config` volume (simplest — copy both files,
or the volume's backing directory, on whatever schedule you back up
elsewhere).

```
docker run --rm -v vakt-config:/config -v "$(pwd)":/backup alpine \
  tar czf /backup/vakt-config-backup.tgz -C /config .
```

**Restore:** extract that archive back into a fresh `vakt-config` volume
before starting the container for the first time on new/replacement
storage.

```
docker run --rm -v vakt-config:/config -v "$(pwd)":/backup alpine \
  tar xzf /backup/vakt-config-backup.tgz -C /config
```

**Why this matters more than it looks like it should:** a browser's push
subscription is cryptographically bound to the VAPID public key it was
created against. If `/config` is lost and `vaktd` starts against an empty
volume, it generates a *new* keypair — every previously-enrolled device's
subscription then silently fails (the push service rejects it), and every
device must be re-enrolled from §5. Restoring the old `vapid.json` (not
just `subscriptions.json`) is what avoids that. If you only ever back up
one file, back up `vapid.json`.

## 7. When a push stops arriving

Work down this list in order:

1. **Check the task's `@state` in the vault file.** If it reads
   `@state(failed)`, a push attempt was rejected and, per G13, Vakt does
   not retry automatically — it waits for a manual clear. **There is
   currently no in-app control for this** (the quick-actions overflow
   menu on a task card is hidden once a task is `failed` — pause/resume/
   skip only apply to non-failed states). The workaround today is to
   edit the file directly: change `@state(failed)` back to
   `@state(active)` and remove the `@reason` directive.
2. **Check the container logs** (`docker compose logs vaktd`) around the
   time the task should have fired, for a delivery error. A `403
   BadJwtToken` from the push service specifically means
   `VAKT_VAPID_CONTACT` isn't a real, resolvable domain — see §3.
3. **Check the device is still enrolled.** A `404`/`410` from the push
   service auto-prunes that device's subscription (G14) — a single dead
   subscription does not fail the household's other devices, but if this
   was the only device, the task fails for lack of any subscription.
   Re-enrol via §5.
4. **Check `/healthz`.** If the daemon isn't up, nothing fires.
5. **Check the task's `@target`.** It must name a real, registered
   module — `ios_notifications` is the only one that exists in the MVP.
   A typo or unsupported name means the dispatch has nowhere to go.

## 8. What Vakt deliberately does not do

These are decisions (SDD.md §5), not bugs:

- **No automatic retry on a failed push.** A transient network drop and
  a permanently dead endpoint look the same to Vakt: `@state(failed)`,
  manual clear required (see §7.1 above, including the current UI gap).
- **No catch-up for a trigger missed while the container was down.** A
  skipped reminder is gone, not queued or fired late on restart.
- **No conflict handling if a file is edited while a task inside it
  fires.** Single-user, home-network use makes this rare; the accepted
  cost is one lost timestamp, not data corruption.
- **One timezone for the whole vault**, set by `VAKT_TZ` — no per-task
  timezones.
- **No support for network or sync-backed vault storage** (iCloud,
  Syncthing, SMB, etc.) — these break reliable file-change events and
  atomic writes. Use local/bind-mounted storage.
- **The Home Screen install step is a real UX cliff.** A household member
  unfamiliar with PWAs may not understand why notifications don't work
  until the app is installed to the Home Screen first — this is explained
  in-app (§5) but is not eliminated.
- **No additional integrations beyond `ios_notifications`.** Home
  Assistant, Slack, MQTT, and generic webhooks are not built, though the
  module interface exists to add them.

---

## For developers

The sections below are developer/tester-facing verification procedures,
not part of installing or operating a deployment.

## Running the device-verification push test

`TestDevice_TriggerDispatchesARealPushToARealSubscription`
(`internal/api/device_test.go`) sends a real Web Push message to a real,
enrolled device. It is build-tagged `device`, so it never runs in
`make test`, `go test ./...`, or CI — it's milestone acceptance, run once
by a human with a real device, not a CI gate (SDD.md §4 M1).

### 1. Get a real subscription

1. Run the real app end to end and enrol a real device through
   `build-enrolment-flow`'s push enrolment dialog.
2. On the running `vaktd` instance's `/config` volume, open
   `subscriptions.json`. It's a JSON object keyed by endpoint; find the
   entry for the device you just enrolled.
3. Copy that one entry's value (`{"endpoint": ..., "keys": {"p256dh": ...,
   "auth": ...}}`) into a new file:

   ```
   internal/api/testdata/device-subscription.json
   ```

   This path is gitignored — see `.gitignore` — because the subscription
   is a secret credential (endpoint + p256dh + auth), is device-bound, and
   expires. Never commit it.

### 2. Point the test at the matching VAPID keypair

A subscription is only valid against the VAPID public key it was created
with. The test needs the **same** `/config` directory the `vaktd`
instance you enrolled against actually uses — not a fresh or different
one, or the push service will reject the message outright.

Set:

```
export VAKT_DEVICE_CONFIG_DIR=/path/to/that/vaktd/config
```

(the directory containing `vapid.json` — e.g. wherever your `/config`
volume is bind-mounted or the `-config-dir` your dev `vaktd` run used).

The test reads only `vapid.json` from this directory; it never opens or
writes that instance's `subscriptions.json`, so running it can't spam
every device an operator has enrolled — it seeds its own scratch store
with just the one subscription from step 1.

### 3. Run it

```
go test -tags device ./internal/api/... -run TestDevice_TriggerDispatchesARealPushToARealSubscription -v
```

Expect a `200` with `TriggerOutcome.Accepted == true`, and the actual
notification landing on the device. Web Push gives no delivery
confirmation (SDD.md §3), so "accepted" is the strongest claim this test
— or any test — can honestly make; watching the notification arrive is
a manual step for whoever runs this.

If the fixture file or `VAKT_DEVICE_CONFIG_DIR` is missing, the test
skips with a message pointing back here rather than failing.

## Performance sanity pass (household-plus scale)

`run-performance-sanity-pass` (M4) checks that nothing in the reactive
cycle is accidentally quadratic at "a household-plus" scale — a few
hundred tasks across a few dozen files — and that SDD.md §1's "fetch the
whole vault, filter in the client" decision still holds there. Not a
benchmark suite: a synthetic-vault sanity check, re-runnable via the tests
below whenever a future milestone wants to recheck it.

### Vault shape measured

Generated by `index.GenerateSyntheticVault(index.HouseholdPlusShape())`
(`internal/engine/index/synthetic.go`):

- 30 markdown files, 14 task lines each -> **410 valid tasks** (some slots
  are deliberately invalid, see below)
- Directive mix, rotated per task: `@schedule` (cron) and `@once`, every
  `@state` (`active`/`paused`/`failed`), plus `@skip_count`,
  `@skip_until`, `@reason`, and `@target` each appearing on a fraction of
  tasks
- 6 deliberately duplicate cross-file `@id`s and 4 deliberately invalid
  `@id`s injected, to exercise `ValidateFile`'s and `indexFileLocked`'s
  diagnostic paths, not just the happy path — the built index reports
  10 diagnostics against 410 tasks

### Measured numbers

Run 2026-09-15, Go 1.27.1 darwin/arm64, Apple M4 Pro laptop — a sandbox
run, not a controlled benchmark environment, but a real baseline for
future comparison:

| Operation | Test | Time |
|---|---|---|
| `Index.Build()` — cold full walk+parse, 30 files / 410 tasks | `TestPerfCheck_BuildAndReconcile` (`internal/engine/index`) | ~1.7ms |
| `Index.ReconcileFile()` — single-file edit, vault already loaded | same test | ~130µs |
| `Scheduler.apply()` + `rearm()` — one task change, 410 tasks already scheduled | `TestPerfCheck_ApplyRearm` (`internal/engine/schedule`) | ~2µs |
| `GET /api/v1/tasks` — real HTTP round trip serving all 410 tasks | `TestPerfCheck_ListTasks` (`internal/api`) | ~3.4ms |

All four numbers are effectively instant relative to any human-perceptible
threshold (`GET /tasks` is asserted under 500ms as a regression bar, not
because it was ever close). SDD.md §1's client-filters-everything decision
holds comfortably at this scale.

### Quadratic-pattern check

`Index.indexFileLocked`'s cross-file duplicate-`@id` check
(`internal/engine/index/index.go`) looks like the one place a per-file
operation might scan every *other* file's claimed ids — but it's a single
`idx.tasks[id]` map lookup, O(1) per task, not a scan. `ReconcileFile`'s
before/after diff is scoped to `idsByPath[relPath]` (this file's own ids
only), not the whole vault. The watcher's debounce/stability logic has no
vault-wide scan in the per-event path (only the fixed-interval rescan
does, which is data-size-independent by design). The scheduler's heap
operations are the standard O(log n) `container/heap` push/remove. No
accidentally quadratic pattern found; nothing was changed.

### Re-running

```
go test ./internal/engine/index/... -run TestPerfCheck -v
go test ./internal/engine/schedule/... -run TestPerfCheck -v
go test ./internal/api/... -run TestPerfCheck -v
```

All three run in well under a second (no `perfcheck` build tag needed —
generating ~400 tasks and timing these four operations is fast enough to
be a plain part of `go test ./...`).
