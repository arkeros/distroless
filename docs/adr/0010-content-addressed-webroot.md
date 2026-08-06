# A content-addressed webroot with one mutable entry document

Every file on a webroot's render path is content-addressed and served `immutable`; `index.html` is the only mutable object left on that path. Publishing therefore becomes: write a set of objects nothing yet references, then overwrite the one document through which they become reachable. Immutability is carried by *placement* — hashed output lives under a prefix that already has an `IMMUTABLE` rule — so `//devtools/build/react_component:cache.bzl` remains the single declaration of the cache policy that [ADR 0009](./0009-frontends-are-served-from-buckets.md) made it.

## Why

A built webroot had eleven stable-named objects out of eighteen: `index.html`, the entry bundle `{app}_bundle/{app}_main.js`, both stylesheets, and the icons. Only the six hashed chunks were content-addressed. That has three costs.

**Revalidation sits on the render path.** The entry bundle is `no-cache` and is the module entry, so a returning visitor pays a conditional request before any JavaScript executes; both stylesheets are render-blocking `<link>`s in `<head>` and are also `no-cache`. ADR 0009 moved these apps off Cloud Run because a cold start was "the one impression that decides whether there is a second" — this is that same argument one layer up. The saving is a client↔edge round trip rather than client↔origin, since Cloud CDN fronts the bucket; smaller than it first appears, still on the critical path.

**A publish has a wide mutable surface.** Twelve objects written concurrently is twelve objects that can be half-written. With one mutable object there is no partial state to reason about: the publish either flips `index.html` or it does not.

**Safety rested on an argument rather than on construction.** A partially-failed publish *was* safe, because every mutable object referenced its peers by a stable URL — but establishing that took a paragraph of reasoning about which references could dangle. Reasoning that subtle survives exactly as long as the person who did it.

ADR 0009 rejected "commit-addressed prefixes with a pointer flip ... as real URL-map complexity for a hobby game." This is not a reversal of that. It obtains most of the atomicity the pointer flip was wanted for — a deploy that becomes visible at one write — while touching no URL map, no bucket versioning and no release GC. The rejected option is still rejected; this is the cheap part of what it offered.

## Considered options

**Placement carries the meaning (chosen).** Hashed output goes under `assets/` and `{app}_bundle/`, both already matched to `IMMUTABLE` by prefix rules. `cache.bzl` gets *shorter*: the exact `{app}_main.js` rule — the subtlest thing in the file, the one that had to precede the prefix rule so the entry did not freeze for a year — deletes outright.

**Hashing derives the policy (rejected).** The step that hashes a file knows it hashed it, so it could emit the immutability directly, making it impossible to hash a file and forget to mark it. Rejected because it spends ADR 0009's central decision — one ordered rule list, declared once, rendered into both the nginx conf and the uploader's JSON — to buy a property that placement already gives. The nginx image is still built and still signed, so the policy genuinely has two consumers, and a derived per-file mapping cannot be rendered as `location` blocks.

The ecosystem is unanimous on the chosen option and it is worth recording why. Vite emits `assets/[name]-[hash]`, Next.js `/_next/static/`, Astro `_astro/`, SvelteKit a directory named, literally, `immutable` — and the deploy recipe shipped with each is a two-line prefix policy. Vite's `manifest.json` looks like the rejected option but is not: it exists so a server can inject the right `<script>` tags, never to decide cache headers.

The safety of placement depends on hashing and placement being one decision rather than two. Vite gets that from a single `assetFileNames` knob. Here, esbuild's `entryNames` sets the name and the directory together, so the bundle inherits it; the StyleX stylesheet is the seam where they could drift, which is why it moves under `assets/` rather than being hashed where it stands.

## Consequences

- **`index.html` can no longer be built at analysis time.** It was an `expand_template` with `/{app}_bundle/{app}_main.js` substituted in as a literal. It becomes an execution-time action reading esbuild's `metafile` for the entry's hashed name. A manifest rather than a glob for `{app}_main-*.js`: a glob's failure mode when it matches twice is a wrong `<script>` tag, where a manifest's is a build error.

- **The metafile must not reach the webroot, and is kept out structurally.** `metafile = True` declares `{app}_bundle_metadata.json` in the esbuild target's default outputs, from which it would flow through the `:{app}` filegroup into both the public bucket and the nginx image — publishing the full module graph, every input path and the source tree layout. Excluding it by pattern alongside `**/*.map` was rejected: sourcemaps are written *inside* the output directory and a filegroup physically cannot select them out, whereas the metafile is a sibling file and can be. "The servable set holds only servable things" is a better invariant than "the servable set is everything, minus a list of regrets" — the latter silently fails to cover the next build byproduct someone adds.

- **A content-addressed name that escapes the immutable prefixes is a bug in two ways at once.** It gets `no-cache` from the default rule, and it lands in the same publish wave as the object that references it, reintroducing the 404 the wave boundary exists to prevent. This is the standing constraint on `cache.bzl`: content-addressed output must be covered by an `IMMUTABLE` rule, which under the chosen option means it must be placed under a prefix that has one.

- **The publish's wave boundary is still required, and gets sharper.** `index.html` names a hashed entry bundle, so the content-addressed wave must still complete before the entry document is written. What changes is the second wave's size — from twelve objects to essentially one — so a client *arriving* mid-publish sees either the previous site or the new one, never a mixture.

  That property is about arriving clients only. A client already on the page holds the previous entry document and its chunk URLs, and hashing does not change what it holds; a lazy route it navigates to is fetched from the old hash long after the deploy. Wave ordering cannot reach it, so the publish retires orphans instead of deleting them and the bucket's lifecycle rule collects them 30 days on — see [ADR 0009](./0009-frontends-are-served-from-buckets.md). Hashing makes the arriving-client story exact; retention is what covers everyone else.

- **Concurrent publishes to one bucket still interleave, and nothing prevents it.** Hashing shrinks the blast radius — two overlapping publishes now race on one object rather than twelve — but a lock would need a leased GCS object with stale-lock recovery, and the failure mode of getting that wrong is a wedged deploy path. Deliberately not built. Each app has its own bucket and its publish is a deploy node ordered after its own root, so overlap requires two concurrent deploy runs.

- **Rollback is unchanged: still a rebuild, not a traffic flip.** Hashing does not add versioning. The previous bytes are still reproduced by `git checkout <sha> && bazel run //apps/x:bucket_push`, which is what ADR 0009 settled on. What is new is that the write which makes that rollback visible is a single one, to `index.html`.

- **The icons and `manifest.webmanifest` stay unhashed.** They are off the render path, and `manifest.webmanifest` → `icon-192.png` is a stable-to-stable reference, which always resolves regardless of publish order. Hashing them would cost a build step for no round trip anyone waits on.
