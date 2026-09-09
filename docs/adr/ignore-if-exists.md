# ADR (BE): cdx resize worker skip-if-warm (`IgnoreIfExists`)

- **Date:** 2026-09-07
- **Status:** Proposed
- **Branch:** TBD (off `main`)
- **Counterpart (gold-http, lm repo):** set `"IgnoreIfExists": true` on warming
  resize triggers published to `asset-delivery-resize`.
- **Rationale:** `ignore-if-exists-rationale.md` — why a worker skip flag over
  always-skip-if-fresh (no flag), why lifted `IsExpired` over duplicating the
  predicate in the worker, and why a freshness check over plain existence.
  **Read it before proposing a change to `Resize` or the freshness rule.**

## 2. Problem

- `Resize` (resize.go:28) always resizes, ignoring any object already at
  `opts.ObjectKey()`.
- We want the option to resize only when no existing file is there.

Requirements:

- `IgnoreIfExists bool` on `ResizeOptions`, default `false` — the current
  "always resize" contract is unchanged for existing messages.
- When `true`, `Resize` returns early (no fetch, no write) when
  `fs.Info(opts.ObjectKey())` reports the object exists and is fresh.
- Missing or stale → proceed to resize (a missing object is just the
  degenerate stale case).
- One freshness rule: `isExpired` lifted to exported `IsExpired` in package
  `asset_delivery`, used by both the worker and `cmd/delivery`
  (`NeedsResizing`, `signHandler`).
- Backward compatible over the wire: the field absent in a legacy message
  deserializes to `false` (zero value); no JSON tag, matching existing fields.

## 3. Strategic Summary

1. `ResizeOptions` (resize-options.go:15) gains `IgnoreIfExists bool`; no
   JSON tag — absent ⇒ `false`, so legacy messages keep the current contract.
2. `Resize` (resize.go:28), when `opts.IgnoreIfExists`, calls
   `fs.Info(opts.ObjectKey())` before any fetch; exists-and-fresh ⇒ `nil` (no
   fetch, decode, encode, or write), missing-or-stale ⇒ falls through to the
   existing path unchanged.
3. `isExpired` (cmd/delivery/server.go:121) lifts to exported `IsExpired` in
   package `asset_delivery`; `NeedsResizing` (:131) and `signHandler` (:183)
   call the exported one — same rule, one definition, no behavior change.
4. The worker (`cmd/resize/server.go:62`) is unchanged — it already
   unmarshals into `ResizeOptions` and calls `Resize`; the new field flows
   through the Pub/Sub JSON on its own.

Counterpart (gold-http, lm repo):

- Sets `"IgnoreIfExists": true` on warming resize triggers published to
  `asset-delivery-resize`.

## 4. Detail

### 4.1 `IgnoreIfExists` field + `Resize` skip

```go
// resize-options.go — one field added; no JSON tag (zero value = false)
type ResizeOptions struct {
	Width          uint
	CacheKey       string
	Location       string
	HashSum        string
	Encoding       string
	Prefix         string
	CacheControl   string
	IgnoreIfExists bool // skip fetch+resize+write when the object is warm + fresh
}
```

```go
// resize.go — guard at the top of Resize; rest unchanged
func Resize(fs FileSystem, opts ResizeOptions) error {
	if opts.IgnoreIfExists {
		info, err := fs.Info(opts.ObjectKey())
		if err == nil && info != nil && !IsExpired(info) {
			return nil // warm + fresh: no fetch, decode, encode, or write
		}
		if err != nil && err != ErrNoFile {
			return &SystemError{Detail: "Could not check if image already exists.", RootError: err}
		}
		// ErrNoFile, or exists-but-stale: fall through and (re)resize.
	}
	// was: body began here
	buf, cc, err := GetImage(opts.Location)
	// …ReaderToImage → ResizeImage → ImageToBytes → fs.Write, unchanged
	return nil
}
```

- Opt-in flag and freshness-over-existence: see rationale §R1, §R3.

### 4.2 Lift `isExpired` → exported `IsExpired`

```go
// file-system.go — moved from cmd/delivery/server.go:121; operates on FileInfo
func IsExpired(info FileInfo) bool {
	control := cachecontrol.Parse(info.CacheControl())
	if control.MaxAge() <= 0 {
		return false // no max-age ⇒ never expires ⇒ always fresh
	}
	return time.Now().After(info.Created().Add(control.MaxAge()))
}
```

- `file-system.go` gains `github.com/marcw/cachecontrol`; `time` already imported.
- `cmd/delivery/server.go` drops its local `isExpired` and the `cachecontrol`
  import (no other user); `time` stays (used by `signHandler`'s
  `time.Now().Unix()`).
- Call sites are dot-imported (`. "github.com/monstercat/asset-delivery"`),
  so bare `IsExpired`:

```go
// cmd/delivery/server.go:136 — NeedsResizing
} else if info != nil && !IsExpired(info) {

// cmd/delivery/server.go:183 — signHandler
if err == ErrNoFile || info == nil || IsExpired(info) {
```

- Lift vs duplicate: see rationale §R2.

### 4.3 Worker — unchanged

```go
// cmd/resize/server.go:62 — already unmarshals the whole struct; field flows for free
var data ResizeOptions
if err := json.Unmarshal(req.Message.Data, &data); err != nil { … }
data.PopulateHash()
// …
if err := Resize(s.FS, data); err != nil { … }
```

- `IgnoreIfExists` is picked up by struct unmarshal and read inside `Resize`.
  No worker change.

### 4.4 Counterpart (gold-http)

```json
// resize trigger published to asset-delivery-resize
{
  "CacheKey": "gold://file/87ab4475-...",
  "Location": "https://storage.googleapis.com/...?sig=...&expires=...",
  "Width": 512,
  "Encoding": "webp",
  "Prefix": "resized",
  "IgnoreIfExists": true
}
```

- Omit the field (or set `false`) for any trigger that must re-encode
  regardless of warmth.

## 5. Expected results

- Test infra: unit tests against the `FileSystem` interface (mockable,
  file-system.go:11) — no live GCS. The "proceeds" paths need an `httptest`
  server because `Resize` calls `GetImage` (a real `http.Get`).
- Ranked test paths:
  1. `IgnoreIfExists=true` + exists-and-fresh → no `Write`, `nil` — the core
     new behavior.
  2. `IgnoreIfExists=true` + missing → fetches + writes (fill).
  3. `IgnoreIfExists=true` + stale → fetches + writes (refresh) — proves
     freshness, not plain existence.
  4. `IgnoreIfExists=false` → always writes (regression; current behavior).
  5. `IsExpired` direct: fresh / stale / no-max-age (never expires) — covers
     the lifted function so `cmd/delivery`'s removal is safe.
- Also: `cmd/delivery` `NeedsResizing` and `signHandler` behave identically
  after the lift — existing `cmd/delivery/sign_test.go` stays green.