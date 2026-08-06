# A path the site does not serve returns 404

The URL map's SPA fallback no longer sets `override_response_code = 200`. It still answers with `/index.html`, so a client-routed app renders its own not-found page, but the status stays 404. Declared client-side routes are materialised as objects in the webroot, so the bucket answers *those* with a genuine 200 without the fallback running at all.

## Why

[ADR 0009](./0009-frontends-are-served-from-buckets.md) set `override_response_code = 200` so that an app rendering a client-side route did not do so under a 404. The reasoning was right about routes and wrong about everything else: the override applies to every path the bucket has no object for, and most of those are paths the site does not serve.

The result was a soft 404 on every unknown URL. A caching proxy stores it. A crawler indexes the not-found page. An uptime check reports healthy. And a missing content-addressed asset — a stale chunk URL from a cached entry document — comes back as `text/html` with a 200, which a module loader cannot use, while every 404 metric reads zero. RFC 9110 is not ambiguous here: 404 is the status for a target resource that does not exist, and 200 asserts that a representation of it follows.

The fix is not to stop serving the shell. The shell is what lets the app render a styled not-found page, and every app declares a `*` route for exactly that. The fix is to stop lying about the status while doing it.

## How a real route still returns 200

A bucket routes by object existence — `/` works because `index.html` is there, not because a rule says so. So a declared route becomes an object: `react_app` renders the entry document to `{route}/index.html` for every path in its route tree. The bucket then answers `/how-to-play/` the ordinary way, and the fallback never runs for it.

Rendered rather than copied, because a route's document knows which route it serves. It preloads that route's chunk — which the entry reaches only by dynamic import, so no client can discover it until the router asks — alongside the entry's static graph, which every document preloads. That only helps a direct hit, since navigating there in-app never fetches the document, but a direct hit is the first impression and this whole stack exists because of one.

This keeps the route table in the build, where routes are already declared, rather than in the URL map. That matters because `//infra/cloud/gcp/lb` is a single root serving six hosts plus the registry: enumerating routes there would make adding a page to one game a Terraform apply on shared routing, with a blast radius across every site. As objects, adding a page stays a `bucket_push`.

It is also the option with *fewer* mechanisms, which is the opposite of how it first looks. Object existence is unavoidable — assets resolve that way and always will, since no URL map can hold them. Given that, expressing routes as objects reuses the mechanism already in play; expressing them as URL-map rules adds a second one alongside it.

`{route}/index.html` rather than a bare `{route}`: the bucket's `main_page_suffix` resolves the directory form, and an extensionless object would be stamped `application/octet-stream` by the closed content-type table ADR 0009 chose — a download prompt instead of a page.

## Consequences

- **A dynamic segment cannot be an object, and is not covered.** `route(path = ":city")` has no finite set of values, so nothing is materialised for it and a bucket-served app would answer it with a 404. The remedy when one appears is a `path_rule` on the host's path matcher — the pattern, not the values — which is what Vercel's `rewrites` and Netlify's `_redirects` express. This is deliberately not enforced at build time: `react_app` is also used for apps never published to a bucket (`//examples/stylex` declares `:city` today), so it cannot know whether the gap matters. The trade is a real one — a silent gap for anyone who adds a dynamic route to a published app — accepted because failing the build for every non-published app is worse.

- **Retention had to be narrowed, or route objects would rot into soft 404s.** [ADR 0010](./0010-content-addressed-webroot.md) retires orphaned objects for 30 days rather than deleting them. Applied to a removed route's object, that would keep answering 200 at a URL somebody deliberately took down, for a month, while the router rendered not-found — reintroducing the exact defect this ADR removes. Retention now covers content-addressed orphans only. The justification for it never reached further: it exists because an old entry document hands out hashed URLs, and a stable-named orphan has no such holder. Stable-named orphans are deleted on the publish that orphans them.

- **A missing asset returns the shell under a 404.** The status is right and a module loader fails cleanly, at the cost of a few KB of HTML nobody parses. Scoping the fallback away from `/app_bundle/*` and `/assets/*` for a bodyless 404 is an optimisation, not a correctness fix, and is not done.

- **Root-level icons stay under the fallback.** `favicon.ico` and friends are stable-named and always present; if one is missing that is the same build bug, and a rule per icon is more URL-map surface than the signal earns.

- **The trailing-slash form is what is guaranteed.** `/how-to-play/` resolves to the materialised object directly. The bare `/how-to-play` relies on Cloud Storage's directory redirect, which is website-mode behaviour the bucket already depends on for `/` — but it is asserted here rather than verified, and is worth confirming against a real deploy.

- **None of this is visible without request logging.** Honest 404s are only useful if something records them, and `log_config` is set on the LB's backend *services* and not its backend *buckets* — so the six game hosts currently have no request logging at all. Fixing the status code without that leaves the same blindness one layer down.
