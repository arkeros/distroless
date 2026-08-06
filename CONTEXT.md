# senku

A Bazel monorepo holding a handful of browser games, the container-image supply chain that builds them, and the Terraform-as-Starlark that deploys them to one GCP project. This file fixes the words used across those three, so the same idea is not called three things.

## Language

### Deployable things

**StaticSite**:
An application with no server — a directory of files a browser fetches.
_Avoid_: static app, SPA (describes the client, not the deployable), frontend

**Runtime**:
An application that runs as a process serving HTTP on `$PORT`.
_Avoid_: service (overloaded — see **Service** below), backend

**Driver**:
The mechanism realising a **StaticSite** or **Runtime** on a cloud — `gcs-cdn`, `cloudrun`. Changing it changes where the bytes are served from, not what the application is.
_Avoid_: origin type, backend kind, platform

**Origin**:
Whatever the load balancer fetches from on a cache miss — a bucket under `gcs-cdn`, a Cloud Run service under `cloudrun`.
_Avoid_: upstream, source

**Kind**:
The type of a bifrost document (`Service`, `CronJob`, `Environment`). Reserved for that — it is *not* the choice between a bucket and a container.
_Avoid_: using "kind" for a **Driver**

### Serving

**Webroot**:
The complete set of files a **StaticSite** serves, at the paths a browser requests them by.
_Avoid_: dist, bundle (the bundle is one part of it), assets

**Cache policy**:
The ordered, first-match-wins rule list assigning `Cache-Control` to each webroot path. Declared once, rendered into both an nginx config and object metadata.
_Avoid_: cache headers, cache rules (as a synonym for the mechanism rather than the declaration)

**Content-addressed**:
A **Webroot** object whose name carries a hash of its own bytes, so the name never resolves to anything else.
_Avoid_: hashed (as a noun), fingerprinted, CAS

**Entry document**:
The one **Webroot** object a client can request without having been told its name, and from which it discovers every **Content-addressed** URL — `index.html`.
_Avoid_: entrypoint (that names the bundle), entry bundle, shell, app shell

**Publish**:
Writing a **Webroot** into a bucket, stamping each object's headers, and deleting what the build no longer produces.
_Avoid_: deploy (a publish is one step of a deploy), sync, upload

**Wave**:
One barrier-separated stage of a **Publish** — every object in a wave is written before any object in the next is begun.
_Avoid_: batch, phase, pass, stage

**SPA fallback**:
Serving `/index.html` with a 200 for a path the **Origin** has no object for, so a client-routed application can resolve the route itself.
_Avoid_: 404 rewrite, catch-all

### Deploying

**Deploy DAG**:
The graph of deploy nodes derived from the build graph — no hand-maintained list. Membership is the rule class; edges come from `ref()`s.
_Avoid_: deploy list, pipeline

**Deploy node**:
One operation in the **Deploy DAG** — a Terraform root, an image push, a bucket publish.
_Avoid_: step, stage, task

**Root**:
A Terraform root: one state file, one `terraform apply`.
_Avoid_: stack, module, workspace

**Service**:
A Cloud Run service specifically. Not a synonym for **Runtime** (which is driver-agnostic) and never a **StaticSite**.
_Avoid_: using "service" for any deployable

## Relationships

- A **StaticSite** or a **Runtime** is realised by exactly one **Driver**, which determines its **Origin**
- A **StaticSite** compiles to a **Webroot** and a **Cache policy**; both are rendered from one declaration
- A **Publish** writes a **Webroot** to a bucket **Origin**; a **Root** creates the bucket, and the publish follows it
- A **Publish** applies its **Wave**s in order — **Content-addressed** objects, then the **Entry document**, then the deletions
- A **Content-addressed** object is unreachable until an **Entry document** names it, which is what a **Wave** boundary buys: the new names are all in place before anything hands them out
- The load balancer routes a host to one **Origin**, and adds the **SPA fallback** only when that origin's **Driver** is `gcs-cdn`
- Every **Root**, image push and **Publish** is a **Deploy node**; their edges are the `ref()`s between them

## Example dialogue

> **Dev:** "The games moved from Cloud Run to buckets — so they're a different **Kind** now?"
> **Domain expert:** "No. They were **StaticSite**s the whole time — a directory of files with no server. Running them under nginx in a container was a **Driver** choice, and `gcs-cdn` is a different one. Nothing about the application changed."
> **Dev:** "And the registry?"
> **Domain expert:** "That one's a **Runtime**. It's a real process; it can't be a **StaticSite** on any driver."
> **Dev:** "Why does only the games' host get the **SPA fallback**, then?"
> **Domain expert:** "Because a bucket **Origin** can't answer for a path it has no object at — it 404s. A **Runtime** answers its own unknown paths, so it doesn't need one."
> **Dev:** "So a **Publish** overwrites the whole **Webroot** at once?"
> **Domain expert:** "It can't — a bucket has no transaction. It writes in **Wave**s instead. Every **Content-addressed** object goes first, and none of them is reachable yet, because the only thing that hands out those names is the **Entry document** and that still has yesterday's bytes. Then the **Entry document** flips, and the whole new site becomes reachable in one write."
> **Dev:** "And if the first wave dies halfway?"
> **Domain expert:** "Then the **Entry document** never flips and the site is still yesterday's, coherently. That's the point of the boundary — a **Publish** either lands or it doesn't, rather than leaving a page that names objects nobody uploaded."

## Flagged ambiguities

- "kind" was used for both the bifrost document type (`Service`, `CronJob`) and the LB's bucket-vs-container choice — resolved: the latter is a **Driver**, and `LB_BACKEND` names it `driver`.
- "service" was used for Cloud Run services, for **Runtime**s generally, and for any deployable including the **StaticSite**s — resolved: it means a Cloud Run service and nothing else. The games' `SERVICE_NAME` was renamed to `SITE_NAME`, since it names a GAR repository and a bucket, not a service. `SERVICE_NAME` survives only in `//oci/cmd/registry`, where it is one.
- "deploy" was used for both the whole DAG walk and the bucket upload alone — resolved: the upload is a **Publish**.
- "unhashed" was used as though it described `index.html` alone, when a built webroot also serves stable-named stylesheets, the entry bundle and the icons — 11 of `table-for-two`'s 18 objects. Resolved: **Entry document** means `index.html` and nothing else, and everything on the render path is being made **Content-addressed** so that the two words finally describe the same split.
