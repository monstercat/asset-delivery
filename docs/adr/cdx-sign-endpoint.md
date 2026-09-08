# ADR (BE): cdx authed `/sign` endpoint + `CacheKey`-decoupled storage

- **Date:** 2026-09-07
- **Status:** Proposed
- **Branch:** `cdx-sign-endpoint-adr` (off `main`)
- **Counterpart (gold-http, lm repo):** gate on the cover route; call cdx
  `/sign`; trigger resize via `asset-delivery-resize`; signed-original
  fallback; warming feed; `CacheKey` scheme `gold://file/<fileID>`; HMAC
  shared-secret holder.
- **Rationale:** `cdx-sign-endpoint-rationale.md` — why `/sign`+`CacheKey`
  over a user-facing capability sig, gold-http byte-proxying,
  `Cache-Control: must-revalidate`, a Pub/Sub request/reply, gold-http
  doing its own GCS stat, removing the redirector, and a private bucket
  + GCS signed URLs. **Read it before proposing a serving or gating
  change.**

## 2. Problem

- gold-ui serves gated images through cdx's public redirector; hits
  `301` to public-read GCS objects (cmd/delivery/server.go:91); once
  resized, anyone hitting cdx gets the bytes — anon sees gated images.
- the gate lives in gold-http (different domain, holds the session); cdx
  can never hold the user's auth, so cdx can't gate the redirect.
- cdx is a shared service — another consumer uses the public redirector
  — so it can't be removed or made auth-only.
- the fix moves *serving* under gold-http's gate: gold-ui routes gated
  images through the gated cover route, which calls cdx `/sign` for the
  resized object's URL. The gate is two parts:
  - the cover route (session + intricate auth) is the only way to reach
    `/sign` (HMAC-authed);
  - the resized object is stored at `sha1(CacheKey)`, where `CacheKey`
    is a server-side identity (`gold://file/<fileID>`) not present in
    any public response. anon only knows the public cover-route URL,
    which hashes to a *different*, empty key — so the redirector misses
    and `307`s to the gated cover route. The CacheKey-keyed object is
    unguessable to anon and only revealed by `/sign`.
- decoupling: today `ObjectKey = resized/<sha1(Location)>/<width><ext>`
  (resize-options.go:37, :30); the storage key is the source URL, so
  whoever re-derives the key must reconstruct the same public `/api`
  source URL — which gold-http can't (the LB strips `/api`). The key
  must be a stable identity (`CacheKey`), not the fetch URL.
- the resized bucket stays public-read; the CacheKey path is the gate,
  not object privacy.

Requirements:

- `cmd/delivery` keeps its public browser flow; add authed
  `GET /sign/<cacheKey>?width=&encoding=&signature=&expires=` →
  `{url}` if the object exists, else `404`; no creation.
- `/sign` authed via HMAC-SHA256 signed query (shared secret);
  invalid/expired → blank `404`.
- `ObjectKey` derives from `sha1(CacheKey)` when `CacheKey` is present;
  else `sha1(Location)` (legacy path retained for the shared consumer).
- `ResizeOptions` gains `CacheKey`; `Location` becomes fetch-only.
- resize trigger is gold-http via `asset-delivery-resize`; `/sign` never
  creates.

## 3. Strategic Summary

1. **cdx redirector** (`cmd/delivery`): public browser flow unchanged;
   new authed `GET /sign/<cacheKey>?width=&encoding=&signature=&expires=`
   → `FS.Info(ObjectKey)` → on hit `200 {url: FS.ObjectURL(ObjectKey)}`,
   on miss/invalid-auth `404`; no creation.
2. **`/sign` auth**: `sig = hex(HMAC-SHA256(SHARED_SECRET, cacheKey +
   ":" + width + ":" + encoding + ":" + expires))`; cdx recomputes +
   checks `expires > now`; else blank `404`.
3. **`ResizeOptions`** (resize-options.go:15) gains `CacheKey string`;
   `PopulateHash` hashes `CacheKey` when present, else `Location`;
   `ObjectKey`/`Location`/`Resize` otherwise unchanged.
4. **cdx worker** (`cmd/resize`): unchanged; on `CacheKey`-bearing
   messages `Location` is a signed-GCS original; stores at
   `ObjectKey(CacheKey)`.
5. **gold-http (counterpart)** gates its cover route (session +
   intricate auth), calls cdx `/sign` (HMAC-signed) → on `200` `302` to
   the returned URL; on `404` publishes
   `{CacheKey, Location, Width}` to `asset-delivery-resize` and `302`
   to the signed original (full-size fallback); warming pre-publishes.

## 4. Detail

### 4.1 `/sign` endpoint (cmd/delivery)

- new path on the redirector; the existing `ServeHTTP` browser flow is
  untouched.
- route via a `http.ServeMux` in `cmd/delivery/main.go`; cacheKey comes
  from the path, the rest from the query.
- the returned URL is the public `ObjectURL` at the CacheKey-keyed path;
  the gate is that the CacheKey path is unguessable by anon and `/sign`
  is the only thing that reveals it (HMAC-gated).

```go
// cmd/delivery/main.go — wrap the existing handler with a mux
mux := http.NewServeMux()
mux.HandleFunc("/sign/", server.signHandler) // new
mux.Handle("/", server)                      // existing public flow
http.ListenAndServe(address, mux)
```

```go
// cmd/delivery/server.go
const SignSecretEnv = "CDX_SIGN_SECRET"

// GET /sign/<cacheKey>?width=512&encoding=webp
//     &signature=<sig>&expires=<unix>
// 200 {url} if the resized object exists and is fresh; else 404.
// Never creates. Invalid/missing/expired auth -> blank 404 (reveals
// nothing).
func (s *Server) signHandler(w http.ResponseWriter, r *http.Request) {
	cacheKey := strings.TrimPrefix(r.URL.Path, "/sign/")
	q := r.URL.Query()
	widthU, err := strconv.ParseUint(q.Get("width"), 10, 32)
	if err != nil || widthU == 0 || widthU > MaxImageDimension {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	width := uint(widthU)
	enc := q.Get("encoding")
	sig := q.Get("signature")
	expires, _ := strconv.ParseInt(q.Get("expires"), 10, 64)

	secret := []byte(os.Getenv(SignSecretEnv))
	if !verifySignSig(secret, cacheKey, width, enc, sig, expires) ||
		time.Now().Unix() > expires {
		w.WriteHeader(http.StatusNotFound) // blank 404
		return
	}

	opts := ResizeOptions{
		CacheKey: cacheKey,
		Width:    width,
		Encoding: enc,
		Prefix:   s.Prefix,
	}
	opts.PopulateHash() // hashes CacheKey

	info, err := s.FS.Info(opts.ObjectKey())
	if err == ErrNoFile || info == nil || isExpired(info) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": s.FS.ObjectURL(opts.ObjectKey()),
	})
}

// verifySignSig checks sig (hex) == HMAC-SHA256(secret,
// cacheKey:width:enc:expires). Covers every param a caller can
// tamper with.
func verifySignSig(secret []byte, cacheKey string, width uint,
	enc, sig string, expires int64) bool {
	if len(secret) == 0 || sig == "" {
		return false
	}
	msg := cacheKey + ":" +
		strconv.FormatUint(uint64(width), 10) + ":" +
		enc + ":" + strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	return hmac.Equal(mac.Sum(nil), got)
}
```

- `isExpired` (server.go:116) reused unchanged.
- `ObjectURL` (file-system-gcloud.go:63) reused — no new FS method.
- `encoding` is required for `/sign` (no `Location` to derive an
  extension from).

### 4.2 `CacheKey` + `PopulateHash` (resize-options.go)

- one new field; one function changes; `ObjectKey` unchanged.

```go
type ResizeOptions struct {
	Width        uint
	CacheKey     string // stable identity; hashed when present
	Location     string // fetch URL; hashed when CacheKey empty (legacy)
	HashSum      string
	Encoding     string
	Prefix       string
	CacheControl string
}

// PopulateHash hashes CacheKey when present (gold-http path); else
// Location (legacy browser-url path, retained for the shared
// consumer).
func (opts *ResizeOptions) PopulateHash() {
	key := opts.CacheKey
	if key == "" {
		key = opts.Location
	}
	hash := sha1.New()
	hash.Write([]byte(key))
	sum := hash.Sum(nil)
	opts.HashSum = fmt.Sprintf("%x", sum)
}
```

- `ObjectKey()` (resize-options.go:37) unchanged:
  `fmt.Sprintf("%s/%s/%d%s", opts.Prefix, opts.HashSum,
  opts.Width, opts.DesiredEncoding())`.
- `NewResizeOptionsFromQuery` (resize-options.go:48) unchanged — the
  browser flow still parses `url=` into `Location`; `/sign` builds
  `ResizeOptions` directly, not from a query.
- backward-compatible: messages without `CacheKey` (the shared
  consumer) keep hashing `Location` — same `ObjectKey` as today, same
  GCS objects, no migration.
- switchover: old messages still in flight when the change lands
  (`CacheKey` empty) use the url (`Location`) as the cache key — same
  `ObjectKey` as before the change, so nothing in the queue or the
  shared consumer's flow breaks.

### 4.3 Gold-http ↔ cdx contract (counterpart; not in this repo)

- `CacheKey`: `gold://file/<fileID>` — stable; gold-http constructs
  from the fileID; no host, no `/api`. The scheme is server-side and
  must not appear in any public response (it is the gate).
- `Location`: a short-lived signed-GCS original URL gold-http mints
  (lm: `getGoldImageSignedUrl` → `c.GetFS().SignedUrl`,
  route-file.go:157).
- `SHARED_SECRET` (`CDX_SIGN_SECRET`): held in both services' env.
- the resized bucket stays public-read; the `sha1(CacheKey)` path is
  unguessable to anon, so public readability does not leak gated
  objects.

```json
// resize trigger, published to "asset-delivery-resize":
{
  "CacheKey": "gold://file/87ab4475-...",
  "Location": "https://storage.googleapis.com/...?sig=...&expires=...",
  "Width": 512,
  "Encoding": "webp",
  "Prefix": "resized"
}
```

```
// warmth check, gold-http -> cdx:
GET /sign/gold://file/87ab4475-...?width=512&encoding=webp
    &signature=<sig>&expires=<unix>
  signature = hex(HMAC-SHA256(SHARED_SECRET, cacheKey + ":"
    + width + ":" + encoding + ":" + expires))
// 200 -> { "url": "<public ObjectURL at sha1(CacheKey)>" }
```

- `/sign` does not create; on `404` gold-http publishes the resize
  trigger and falls back to the signed original.

## 5. Expected results

- test infra: unit tests against the `FileSystem` interface
  (mockable, file-system.go:11) — no live GCS for `/sign` or
  `PopulateHash`/`ObjectKey` tests. Existing `resize_test.go` and
  `cmd/delivery/server_test.go` stay green (public flow unchanged).
- ranked test paths:
  1. `/sign` returns the URL when the object exists and is fresh;
     `404` when missing; `404` when `isExpired`.
  2. `/sign` returns blank `404` for missing/invalid/expired signature
     — no existence leak across auth states.
  3. `PopulateHash` hashes `CacheKey` when set, `Location` when not
     — legacy messages produce the same `ObjectKey` as today
     (backward-compat).
- also: a round-trip test — gold-http-style message (`CacheKey` +
  signed-GCS `Location`) resizes, stores at `ObjectKey(CacheKey)`, and
  a subsequent `/sign` for the same `CacheKey` returns the `ObjectURL`
  for that object.