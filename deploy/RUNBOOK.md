# Operator runbook

This accumulates as decisions are made (SDD.md §7) rather than being
written from memory at the end. First entry below.

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
