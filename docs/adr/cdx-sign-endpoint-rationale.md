# cdx `/sign` endpoint + `CacheKey` — rationale

- **Design:** `cdx-sign-endpoint.md`
- **Date:** 2026-09-07

Already measured and rejected — do not re-propose without new
numbers:

- **R1 — user-facing capability sig (HMAC in the cdx URL):** cdx is
  cross-domain (no session); forces sig transport gold-http→gold-ui
  (round-trip / payload enrichment / a re-deriving sign route) — all
  rejected; and it gates a shared public service.
- **R2 — gold-http proxies cdx resized bytes:** byte proxying is the
  cost gold-http 302s-to-signed-GCS to avoid; reintroduces it on every
  gated view.
- **R3 — `Cache-Control: must-revalidate`:** freshness directive, not
  access control; can't gate who sees the object.
- **R4 — Pub/Sub request/reply for the warmth check:** Pub/Sub is
  fire-and-forget; no native reply; request+response topics +
  correlation IDs > one HTTP `GET /sign`.
- **R5 — gold-http stats GCS + signs directly:** forces gold-http to
  hold cdx's bucket + `ObjectKey` + re-derive the `/api` source URL
  (LB strips `/api`); duplicates cdx internals in gold-http.
- **R6 — remove the redirector:** cdx is a shared service; another
  consumer uses the public `301`-to-GCS flow.
- **R7 — bearer-header or JWT auth on `/sign`:** bearer = long-lived
  token, no per-request binding; JWT = lib + claims, overkill for one
  service-to-service call.
- **R8 — private bucket + GCS signed URLs (considered, deferred):**
  would make the resized object unreachable except via a time-limited
  signed URL, closing the leaked-URL-permanent-access gap. Set aside:
  needs a GCS SA signing key in cdx (or IAM SignBytes) + private
  objects, when the unguessable CacheKey path already keeps anon out.
  Accepted tradeoff: a leaked `ObjectURL` grants permanent access to
  that one resized cover (low-sensitivity, eventually public).

Measurement conditions: design analysis against live code —
`cmd/delivery/server.go`, `resize-options.go`, `resize.go`,
`file-system-gcloud.go` (this repo); `cmd/gold-http/route-file.go`,
`route-release.go` (lm repo); prod Cloud Run request logs confirming
the `/api` LB-strip and the cover-route `302`→signed-GCS pattern.

## R1. Why not a user-facing capability sig

- the leak is cdx `301`-ing anyone to a public resized GCS object
  (server.go:91).
- a sig in the cdx URL (`cdx?url=…&sig=…`) would gate the redirect,
  but cdx is on `monstercat.com` and the session is on
  `soundscout.com` — cdx can't verify the user.
- so gold-http must mint the sig and gold-ui must carry it. Every
  transport option failed:
  - per-image sig round-trip — extra request per `<img>`.
  - payload enrichment — the release/track payload has no image info
    and a release can have several images.
  - standalone `sign-image?url=` route — re-derives the intricate
    per-image auth (tier/subscription/label) from a bare URL;
    duplicates the cover route's gate logic.
- and cdx is shared; gating its public flow affects the other
  consumer.
- `/sign` + `CacheKey` sidesteps it: the gate stays in gold-http's
  cover route; cdx only hands a signed URL to an
  already-authenticated service caller.

## R2. Why not gold-http proxying resized bytes

- gold-http `302`s originals to signed GCS (route-file.go:175)
  precisely to avoid proxying bytes.
- proxying resized bytes from cdx reintroduces that cost on every
  gated cover view.
- `/sign` returns a signed GCS URL the browser fetches directly — no
  proxying, same `302`-to-signed-GCS pattern gold-http already uses
  for originals.

## R3. Why not `Cache-Control: must-revalidate`

- `must-revalidate` is a freshness directive: when stale, revalidate
  with origin before serving.
- it does not gate who sees the object; a fresh entry is served to
  anyone.
- the leak is access-control, not freshness — `must-revalidate` is
  the wrong tool (CDNs use signed URLs / signed cookies / edge tokens
  for auth, not `must-revalidate`).

## R4. Why not Pub/Sub request/reply for the warmth check

- Pub/Sub is fire-and-forget; no native reply.
- a request/reply needs a request topic + response topic +
  correlation IDs + a subscriber that publishes the reply — more
  moving parts than one HTTP `GET /sign`.
- Pub/Sub stays for the one-way resize trigger (its strength);
  `/sign` is the synchronous existence/sign check.

## R5. Why not gold-http stats GCS + signs directly

- would need gold-http to hold objectReader + signing on cdx's
  resized bucket, know the `ObjectKey` formula, and re-derive the
  storage key.
- today `ObjectKey = sha1(Location)` (resize-options.go:37), so
  gold-http would have to reconstruct the same public `/api` source
  URL — the LB strips `/api`, so it can't (the "wrong URL" in
  gold-http's source is exactly this).
- decoupling via `CacheKey` removes the `/api` reconstruction, but
  gold-http would still duplicate cdx's bucket + `ObjectKey`
  internals.
- `/sign` keeps the bucket + `ObjectKey` + signing in cdx (where
  they live); gold-http just calls.

## R6. Why not remove the redirector

- cdx's redirector is a shared service; another consumer uses the
  public `?url=&width=` `301`-to-GCS flow.
- removing it or making it auth-only breaks that consumer.
- so the public flow stays; `/sign` is purely additive.

## R7. Auth on `/sign`: HMAC-signed query vs bearer vs JWT

- bearer header (shared secret): simple, but a long-lived token in
  both envs; no per-request binding to the params.
- JWT: standard claims + verification lib; overkill for a single
  service-to-service call.
- HMAC-signed query (`/sign/<cacheKey>?signature=&expires=`):
  - each call self-authorizing and short-lived (`expires`).
  - covers `cacheKey + width + encoding + expires` — tamper-proof
    params.
  - mirrors gold-http's existing `SignedUrl` style — consistent
    with code already there.
- symmetric (cdx holds the secret): acceptable — cdx already holds
  the GCS signing key for the resized bucket, so the blast radius is
  unchanged. Asymmetric would let cdx hold only a public key, but
  adds a key pair for marginal gain here.
- invalid/missing/expired → blank `404`, so the endpoint doesn't
  reveal it exists across auth states.

## R8. Why not a private bucket + GCS signed URLs

- the resized bucket is public-read today; the leak is that cdx
  `301`s anyone to the public `ObjectURL` at `sha1(Location)` — and
  `Location` is the public cover-route URL, so anon can construct it.
- `CacheKey` fixes that root: the gated object lives at
  `sha1(CacheKey)` where `CacheKey = "gold://file/<fileID>"` is a
  server-side scheme not in any public response. anon only knows the
  cover-route URL, which hashes to a different, empty key → the
  redirector misses → `307` → gated cover route → `401`. The
  CacheKey-keyed path is unguessable, so a public bucket does not
  leak it.
- making the bucket private + returning GCS signed URLs would add a
  second layer: even a leaked URL expires. But it costs a GCS SA
  signing key in cdx (or `SignBytes` via IAM + the SA email), and
  private objects — machinery the unguessable CacheKey makes
  unnecessary for the threat anon actually poses.
- accepted tradeoff: a leaked `ObjectURL` (e.g. from an authorized
  user's browser network log) grants permanent access to that one
  resized cover until the object is deleted. Covers are
  low-sensitivity and eventually public, so this is acceptable now.
  Revisit with private objects + signed URLs (R8) if that changes.