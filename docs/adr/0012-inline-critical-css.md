# Stylesheets are inlined into the document, not linked from it

Both stylesheets a `react_app` produces — open-props `normalize.min.css` and the collected StyleX sheet — are inlined into every document `html_codegen.mjs` renders. No CSS URL is served: the webroot contains no `.css` object at all, and the content-addressing that [ADR 0010](./0010-content-addressed-webroot.md) gave the stylesheets is removed along with the requests it existed to make cheap.

## Why

ADR 0010 observed that "both stylesheets are render-blocking `<link>`s in `<head>` and are also `no-cache`", and fixed the second half of that sentence: hashing them bought the `immutable` header, so a returning visitor stopped paying a conditional request for bytes that had not changed. The first half was left standing. A `<link>` is still a request, and `immutable` only helps a client that already has the file.

The cost is a round trip, and it is strictly sequential. `index.html` is `no-cache`, so every visit begins by asking the origin for it. Only when that response arrives can the preload scanner see the `<link>` and start the CSS request — and first paint waits on that second trip, because a render-blocking stylesheet is exactly the thing the browser refuses to paint without. On a phone on mobile data, one edge round trip is worth more than the few kilobytes it fetches. This is [ADR 0009](./0009-frontends-are-served-from-buckets.md)'s argument about cold starts and [ADR 0010](./0010-content-addressed-webroot.md)'s about revalidation, applied to the one remaining request on the render path that neither removed.

Inlining removes the trip rather than making it cheaper. The bytes arrive with the document that needs them, and the document was already being fetched.

The measured trade is better than the framing suggests, because the bytes were never the expensive part:

| app | before (gzip, 3 responses) | after (gzip, 1 response) |
| --- | --- | --- |
| napkin-battle | ~6.5 KB | 6.4 KB |
| dino-meteor | ~4.1 KB | 4.0 KB |

Nothing is duplicated, so the totals barely move — and compressing one stream beats compressing three, which is why the "after" column is marginally *smaller*. What changes is that it is one response instead of three. Uncompressed, `index.html` grows from 1.2–2.7 KB to 13.5–24.4 KB; that number is the one that looks alarming and the one that never travels.

Both columns assume the response is compressed, which is true only because the CDN does it: a bucket serves the bytes it stores and `bucket_push` stores them raw, so the figures above depend on `compression_mode = "AUTOMATIC"` on the backend buckets in `//infra/cloud/gcp/lb:defs.bzl`. That setting is also what makes the growth in raw size safe to ignore, and the two interact once more: Cloud CDN does not compress below 1 KiB, and inlining lifted every app's `index.html` clear of that floor — `cluedo-bayes` was previously at 1.2 KB, close enough that a small edit decided it.

## Considered options

**Inline both sheets (chosen).** The whole of the render-blocking CSS arrives with the document.

**Inline the StyleX sheet, keep normalize linked (rejected).** Superficially the best of both: normalize is the larger half (9.0 KB raw, 2.3 KB gzipped), it is byte-identical across all six apps, and it changes only on an open-props bump — a perfect candidate for a long-lived cached copy. It fails on the arithmetic. The six apps sit on six different hostnames, and browsers have partitioned the HTTP cache by origin for years, so there is no shared copy to hit; each app would cache its own. That leaves one linked stylesheet still on the render path — and one linked stylesheet costs the same round trip as two, since they are fetched in parallel. The option keeps the entire cost of the thing this ADR exists to remove, in exchange for saving 2.3 KB on repeat visits to a document that is `no-cache` anyway.

**Extract genuinely critical CSS, inline that, load the rest async (rejected).** The standard answer at scale, and the right one when the sheet is 100 KB. Here the largest sheet is 13 KB raw and 3.2 KB gzipped. Splitting it needs a critical-path extractor in the build, a definition of "above the fold" for a canvas that fills the viewport, and a second loading mechanism to get the remainder in without blocking — machinery whose failure mode is a flash of unstyled content, bought to defer a few kilobytes that are already in flight. Reconsider if a sheet ever grows past the point where its bytes cost more than a round trip; nothing here is close.

**Preload the stylesheet (rejected).** `<link rel="preload">` in the document cannot beat the `<link rel="stylesheet">` in the same document — the preload scanner finds both in the same pass. It would need `Link:` response headers or early hints from the origin, which is a bucket behind Cloud CDN and has no request-time logic to emit them.

## Consequences

- **CSS stops being content-addressed, and the machinery for it deletes.** `hash_assets` over the stylesheets, the `_css_dir` and `_css_manifest` filegroups, the `--css-manifest` argument and `resolveCss` are all gone. `assets/` now means asset_pipeline output and nothing else, which is a narrower and more honest reading of the prefix than ADR 0010's "two producers, one directory". ADR 0010's standing constraint — content-addressed output must be placed under a prefix with an `IMMUTABLE` rule — is untouched; there is simply one less thing subject to it.

- **`--css` now names a file to read rather than a basename to look up.** `generate()` takes stylesheet *contents*, so it stays a pure function of its inputs and the reads happen in `main`.

- **`</style>` in a stylesheet is now a build failure.** Inlining moves CSS into HTML parsing context, where the tokenizer ends the block at `</style` — case-insensitively, whitespace permitted before the `>`. `content: "</style>"` is legal CSS, so this is reachable input, and its failure mode is the remainder of the sheet rendering as body text under a 200. `html_codegen` rejects it rather than escaping it: neither sheet has ever contained the sequence, and a build error is a better answer than a transform whose correctness nobody will revisit.

- **Each route document carries its own copy.** A `react_app` renders one document per declared route, and the CSS is in all of them. They are never fetched together — a client gets exactly one, and in-app navigation fetches none — so this costs bucket storage, not bandwidth.

- **Repeat visits re-download the CSS only when the document changes.** `index.html` is `no-cache`, which is revalidation and not refetching: an unchanged document is a 304 with no body, and the inlined CSS rides only on the responses that had changed anyway. A deploy that touches one component now invalidates the whole document rather than one hashed stylesheet — but the document was already invalidated by that deploy, because it names the entry bundle's new hash.

- **The devserver still links its stylesheets, and should.** It serves them from `/{basename}` and is unaffected by this change. A separately fetchable sheet is what lets a CSS edit reload without regenerating the document, and nothing the devserver serves is published — the same reasoning ADR 0010 used for keeping it on the unhashed pair.

- **The `.css` rule in `cache.bzl` no longer matches anything a `react_app` emits.** It stays: it is the policy for any `.css` an app ships through `asset_pipeline` or `extra_srcs`, and deleting it would leave such a file on the `REVALIDATE` default by accident rather than by decision.
