# ADR rationale: cdx resize worker skip-if-warm (`IgnoreIfExists`)

- **Design:** `ignore-if-exists.md`
- **Date:** 2026-09-07
- No measurements — alternatives are design-shape, evaluated by contract and
  semantics, not performance.

| idea | § | why not |
|---|---|---|
| always-skip-if-fresh (no flag) | R1 | flips the worker's default contract for every consumer; no way to force a re-encode |
| duplicate `isExpired` in the worker | R2 | two "stale" definitions can drift; worker and `/sign` disagree on warmth |
| plain existence (no freshness) | R3 | never refreshes an expired object; worker and `NeedsResizing` disagree on "needs resize" |

## R1. Opt-in flag vs always-skip-if-fresh

Always-skip-if-fresh makes the worker check `fs.Info` + `IsExpired` on every
message and skip when warm, with no flag. Rejected because:

- it changes the worker's default contract from "always resize" to "resize
  unless fresh" for every consumer, including the shared browser-flow
  consumer — a behavioral change for messages that never opted in;
- a publisher that must re-encode regardless of warmth (encoding/quality
  change, a forced refresh) has no way to bypass it;
- a silently-dropped resize is harder to distinguish from a bug than an
  explicit `IgnoreIfExists=true` skip.

The opt-in flag keeps `false` as the byte-for-byte current behavior and lets
only the warming publisher request skip semantics. Backward compatible by
default; publisher controls.

## R2. Lift `IsExpired` vs duplicate vs delivery-side-only

- **Duplicate** the predicate in package `asset_delivery`: two definitions of
  "stale" that can drift; the worker and `/sign` would eventually disagree on
  whether the same object is warm.
- **Leave freshness delivery-side only**, use plain existence in the worker:
  rejected on semantics (§R3), not just structure.
- **Lift** `isExpired` (cmd/delivery/server.go:121) to exported `IsExpired` in
  package `asset_delivery`: one rule, used by `Resize`, `NeedsResizing`, and
  `signHandler`. Can't drift. The lift is mechanical — the body operates on
  the `FileInfo` interface already defined in `file-system.go`, so no new
  abstraction, just a package move + the two `cmd/delivery` call sites
  switching to the dot-imported name.

## R3. Freshness vs plain existence

Plain existence skips whenever the object is present, even expired. Freshness
skips only when present *and* not `IsExpired`; a stale object is re-resized.

- Plain existence never refreshes an object whose `Cache-Control` max-age has
  elapsed. The warming path is the thing that refreshes; if it skips stale
  objects, they stay stale until something else forces a re-resize — a
  stuck-stale object.
- Freshness matches `NeedsResizing`'s "needs resize = missing OR expired", so
  the worker and the delivery server agree on warmth. The worker's skip
  condition becomes exactly the delivery server's "no need to resize"
  condition.
- "A stale image is one that doesn't exist anyways" (user): for the skip
  decision, missing and stale are the same condition — both mean "needs
  resize." `IsExpired` returns `false` for no-max-age objects (never expire),
  so those are treated as fresh and skipped, which is correct: no expiry =
  always warm.