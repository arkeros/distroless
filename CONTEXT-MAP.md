# Contexts

Most of this repo speaks one language: how things are built, published and
served. That vocabulary lives in [CONTEXT.md](./CONTEXT.md) and the decisions
behind it in [docs/adr/](./docs/adr/).

The games are different. A game has a domain of its own — plates, strands,
seats, rounds — that has nothing to say about buckets, and the deployment
vocabulary has nothing to say about it back. Where a game's language has grown
past what its README can hold, it gets its own context.

| Context | Language | Decisions |
| --- | --- | --- |
| Build, publish, serve | [CONTEXT.md](./CONTEXT.md) | [docs/adr/](./docs/adr/) |
| Spaghetti Duel | [apps/spaghetti-duel/CONTEXT.md](./apps/spaghetti-duel/CONTEXT.md) | [apps/spaghetti-duel/docs/adr/](./apps/spaghetti-duel/docs/adr/) |

A game appears here only when it has a `CONTEXT.md`. The others are described
well enough by their README and their rulebook, and inventing a glossary for
them before anyone is confused would be ceremony.
