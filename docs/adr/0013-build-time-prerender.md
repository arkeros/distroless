# Documents ship with their markup already rendered

Every document a `react_app` emits carries the markup for the path it is served at, rendered once in Node at build time by `renderToStaticMarkup`. The client is unchanged: it still mounts with `createRoot`, and the prerendered markup is replaced rather than hydrated.

## Why

First Contentful Paint measures painted *content* — text, an image, an SVG, a drawn canvas. A background colour is not content. Until this change the body of every document was:

```html
<body>
  <div id="root"></div>
  <script type="module" src="/app_bundle/app_main-<hash>.js"></script>
</body>
```

so nothing could paint until the document had parsed, the entry and its preloaded chunks had arrived, ~190 KB of JavaScript had parsed and executed, and React had mounted. On PageSpeed's mobile profile — 1.6 Mbps, 150 ms RTT, 4× CPU throttling — that chain is the whole of FCP, and it measured 2.5 s.

[ADR 0012](./0012-inline-critical-css.md) removed two round trips from that chain and edge compression removed ~205 KB from it. Both were real and both are still worth having, but they shorten the *network* portion of a chain whose floor is JavaScript execution. Nothing on the delivery side can take FCP below the point where React first renders, because before that moment there is nothing on the page to paint.

Rendering the markup at build time removes the dependency rather than shortening it. The document arrives contentful, and paint happens when it parses.

## Considered options

**Prerender for paint, not for hydration (chosen).** `renderToStaticMarkup`, and the client keeps `createRoot`.

**Prerender and hydrate (rejected).** The obvious version, and it would also cut the work React does on mount. It is unavailable here for three independent reasons, any one of which is fatal:

- Routes are `lazy()`. On the client's first render the route module has not loaded, so React renders a fallback where the server rendered the route — a mismatch on every page.
- `pickLocale` reads `navigator.language`. One document is served to every visitor, so a build-time render can only pick the source locale; every non-`es` visitor would mismatch on every string.
- Game state is seeded in components. A prerendered board and a client board are different boards.

Each is solvable — drop code splitting, drop client locale negotiation, lift state — and all three would cost more than hydration is worth for apps whose bundles are already fetched by the time it matters. `renderToStaticMarkup` is chosen over `renderToString` for a related reason: it emits no hydration markers, so React does not warn about a container it thinks came from the server, and the markup is smaller.

**A hand-written static shell per app (rejected).** Cheaper, and it would paint just as early. Rejected because it is a second description of the first frame, in a different language, that nothing keeps in step with the component that renders the real one. It would drift the first time a layout changed, and the failure is invisible — a slightly wrong shell still paints.

## Consequences

- **An app with `runtime_config` is not prerendered.** Those values are injected per deployment precisely so a deployment does not need a rebuild, so `window.__ENV__` does not exist while the build runs and the first `getEnv` in a component throws. The alternative — baking one deployment's values into markup served to all of them — defeats the point of the feature. Such apps build normally with `{{APP}}` resolving to nothing. `//examples/stylex` is the case, and its test asserts the empty `#root` so the opt-out stays deliberate.

- **The canvas games gain nothing, and this is not a bug in the prerender.** `dino-meteor`, `pepper-sweeper` and `spaghetti-duel` draw everything into a `<canvas>`; their prerendered markup is ~135 bytes of wrapper around an empty one, and an undrawn canvas is not contentful. They keep the machinery — it costs a rounding error and would start paying the moment any of them grows DOM chrome — but their FCP needs a drawn first frame or a poster image, which is a separate piece of work. The apps it does help are the DOM ones: `napkin-battle` (12.5 KB of markup), `cluedo-bayes` (11.1 KB), `table-for-two` (2.7 KB).

- **Every route document is rendered at its own path.** `html_codegen` already emitted one document per declared route so a direct hit returns a real 200; each now also carries that route's own markup rather than the index's. A test asserts the two differ, because a wrong-but-present shell paints just as convincingly as a right one.

- **Documents grow, and compression absorbs it.** `napkin-battle`'s index goes from 24.4 KB to 36.9 KB raw, 8.1 KB Brotli. The markup is mostly repeated StyleX class names, which is close to the best case for a compressor. This is only affordable because compression is on at the edge — see the compression section of `//infra/cloud/gcp/lb:README.md`.

- **Growth is now bounded by a test, because past a point it costs a round trip.** A document is the only thing on the render path a client cannot discover early; everything else is named by it. Once a document outgrows the initial congestion window — 10 segments, 14,600 bytes — its tail arrives a round trip after its head, and every URL in that tail with it. The module script sits at the very end of the body, behind the markup this ADR put there, so it is first in line to fall off the edge. That regression has no symptom: the page works, the tests pass, and only a waterfall shows it. `react_app` therefore emits a `_document_budget_test` per app, measured in Brotli at the quality the edge was observed to use. The largest document today is 8.1 KB.

- **A component that throws now fails the build.** Previously a render-time error surfaced in the browser, in whatever state the user's session was in. It now happens once, in Node, with a stack trace — which is why the prerender bundle is not minified. The renderer refuses a route that returns a `Response` rather than markup, so a redirect or a thrown route cannot be written out as an empty body and shipped as a document that renders nothing.

- **The generated render module contains no Node APIs.** The file I/O lives in a sibling `.mjs` that tsc never reads. Otherwise the generated `.tsx` would need `@types/node` in the tsconfig that every `react_component` in the repo shares, to serve one generated file.

- **The devserver serves an empty `#root`.** There is no build step in dev to render into. That matches what the browser has a moment later anyway, and keeping the dev path free of a prerender keeps a component edit to one rebuild.
