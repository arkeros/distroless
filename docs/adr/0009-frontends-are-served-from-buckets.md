# Frontends are served from buckets, not from Cloud Run

Every app in `apps/` is a static SPA. Each one was a Cloud Run service running an nginx image at `min = 0`, behind a serverless NEG, behind a CDN-enabled backend service. Each is now a GCS bucket behind a backend bucket, behind the same CDN.

The image still builds and still pushes to GAR — frontends keep signing, SBOM and provenance — but nothing runs it in production. `//oci/cmd/registry` is unaffected: it is a real server and stays on Cloud Run.

## Why

`min = 0` means the first request after an idle period waits for a container to start. For a game nobody has opened in an hour, that is *every* first request — the one impression that decides whether there is a second.

The alternatives to a bucket are worse in the ways that matter here. `min = 1` per app is five idle instances billed around the clock to serve files that never change. Neither buys anything a bucket does not already give: a bucket has no instance to start, so the question of how long starting takes stops existing.

This mirrors [bifrost's ADR 0009](https://github.com/arkeros/bifrost/blob/main/docs/adr/0009-frontends-are-services-on-a-staticsite.md), which splits `StaticSite` from `Runtime` on the same fact: a static SPA has no server and an SSR application does. Nothing here forecloses that model — `site_gcs` is the `gcs-cdn` driver's shape, arrived at from the concrete side.

## What moved, and where it moved to

An nginx container decides three things per request that a bucket cannot decide at all. Each had to land somewhere explicit.

| nginx did it with | a bucket does it with |
| --- | --- |
| `add_header Cache-Control` per `location` | object metadata, stamped at upload |
| `types` / `default_type` | object `Content-Type`, stamped at upload |
| `try_files $uri $uri/ /index.html` | the URL map's `custom_error_response_policy` |

**Cache policy is declared once and rendered twice.** `//devtools/build/react_component:cache.bzl` holds one ordered rule list. `cache_rules_nginx` renders it into the image's `default.conf`; `cache_rules_json` renders it for the uploader. Since the image survives, the policy has two consumers, and two hand-written copies of it would have drifted — silently, because each origin looks correct on its own.

Order is part of the policy, not an artifact of expressing it. nginx resolves overlapping locations by an implicit precedence (`=` beats `^~` beats regex) that a reader has to know to predict; a bucket has no such rules to fall back on. The list is first-match-wins in both, and a Starlark test pins the order that makes the unhashed entry bundle revalidate rather than freeze for a year.

**Content-Type is a closed table, not `mime.TypeByExtension`.** That function reads `/etc/mime.types` where one exists, so the same build would stamp different types depending on which machine ran the upload. Unknown extensions get `application/octet-stream` rather than a guess: a browser downloads an octet-stream it cannot render, but refuses a module script it believes is the wrong type.

**The SPA fallback gained a property it did not have.** `override_response_code = 200` is what makes the URL map's 404 rewrite a fallback rather than a prettier error page — without it the app renders under a 404, which crawlers and uptime checks read as broken. The bucket's own `not_found_page` would have served the same bytes and kept the 404, so it is not used. The cost is the one `try_files` already had: a genuinely missing asset comes back as HTML with a 200, and the app's router is what tells an unknown route from a broken one.

## Consequences

- **Icons and manifests became cacheable, having previously been uncacheable.** The old nginx conf set `Cache-Control` only inside matching `location` blocks, so a `.png` or `.webmanifest` matched none and went out with no cache directives at all — and Cloud CDN under `USE_ORIGIN_HEADERS` does not cache a response that carries none. Making the policy's default explicit at `server` level, and stamping it on every unmatched object, means those files now revalidate rather than being refetched whole. The default stays `no-cache` rather than a TTL because these URLs are unhashed: a redesign has to be visible on the next load, and a conditional request that 304s is the cheap way to get that.

- **The bucket's contents are not Terraform state.** A `google_storage_bucket_object` per file would put a few hundred objects under management, re-plan on every content-hash change, and still need a rule to enumerate files Starlark cannot see. Terraform owns the bucket, its IAM and its lifecycle; `bucket_push` owns the bytes.

- **`bucket_push` runs *after* its root, where `registry_push` runs before.** Cloud Run reads an image digest back out of the registry, so that push has to precede the apply. A bucket has to exist before anything can be written into it, so this push follows. The asymmetry is in the direction of the dependency, not in the pattern.

- **The LB references the push, not the root.** `published_bucket("//apps/x:bucket_push")` resolves to the bucket's name and carries the deploy edge in the same token — so a backend bucket cannot go live pointing at an empty bucket. Referencing the *root* would have given the same string and allowed exactly the failure [ADR 0008](./0008-derived-terraform-deploy-set.md) was written about: a live route, a bare 404, and nothing anywhere saying why.

- **Stale objects are deleted, and uploads complete first.** Content-addressed chunks would otherwise accumulate forever, one hash per deploy. The ordering is not a preference: a client that has already parsed `index.html` holds URLs for the chunks it names, so a delete that overtook an upload would resolve one of those to nothing.

- **Every object is re-uploaded, never diffed.** The content-addressed majority are byte-identical under a given name, and the rest — `index.html`, the unhashed entry bundle — are precisely the objects whose bytes change under a stable name. A hash comparison would save bandwidth measured in kilobytes and buy a way for a deploy to silently not happen.

- **The buckets are publicly readable, and that is required rather than tolerated.** `google_compute_backend_bucket` reads anonymously; it has no service identity to present. Without `allUsers` the LB gets 403 on every object and the site is unreachable rather than merely uncached. The objects are a public website's compiled assets, so what the grant widens is who can read them by bucket URL as well as by site URL. `storage.publicAccessPrevention` must stay unenforced on `senku-prod` — the LB's 404 bucket already depended on this.

- **The cutover was not zero-downtime, on purpose.** An app's root destroys its Cloud Run service before the LB root repoints that host, and a serverless NEG whose backing service is gone returns a bare 404 — so each app was down for the gap between the two applies. Staging it across three commits (add the bucket origin, flip the URL map, then delete Cloud Run) would have avoided it at the cost of `LB_BACKEND` briefly describing two origins per app. These are games nobody pays for; the window was accepted rather than engineered around.

- **Rollback is a rebuild, not a traffic flip.** Cloud Run revisions could be rolled back in seconds without building anything; a bucket holds one version of the site and has no versioning enabled. Nothing in the webroot path is stamped, so the build is a pure function of sources and `git checkout <sha> && bazel run //apps/x:bucket_push` reproduces the previous bytes. Object versioning was rejected because restoring a coherent site-wide snapshot means restoring a few hundred objects to a timestamp; commit-addressed prefixes with a pointer flip were rejected as real URL-map complexity for a hobby game.

- **The apps lost their runtime service accounts.** Nothing runs, so there is no identity for anything to run as. Any future frontend needing a credential needs a server, which means it belongs back on `Runtime`.

- **`react_static_layer` gained `extra_srcs`, and both origins read it.** The image keeps icons in their own layer because they change on a redesign while the bundle changes every commit; a bucket has no layers and needs the union. The list is named once and assembled once: `:{name}_extras` is the image's layer, `:{name}_webroot` is the bucket's tree, and both come from that one directory. Naming the labels separately on each side — which is how this was first written — is how a bucket ends up serving a file the signed image does not contain, with nothing to say so. Image *content* is unchanged by the restructure; the layer digest moved once, because `copy_to_directory` sorts entries where `app_icons` appended them.

- **The image is not a signature over what is served.** Identical file sets are not a verified link: nothing at publish time checks the objects in the bucket against the image, and the uploader never reads it. What the image gives a frontend is a signed, SBOM'd artifact built from the same sources — not an attestation of the bytes the CDN returns. Getting that property means publishing *from* the pushed image, as bifrost's ADR 0009 describes; it was considered and rejected as more machinery than these games justify.

- **`runtime_config` remains unimplemented, and is now further away.** No app uses it. The bucket path would want the `config.json` bifrost's ADR describes — written as an object at publish time — rather than the `envsubst`-at-container-start the existing `fail()` describes. Whoever needs it first should read that ADR before extending `react_static_layer`.
