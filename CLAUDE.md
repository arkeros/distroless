This is a monorepo with bazel.

use red green TDD

After adding a new Python dependency, run `.tools/repin` to update lock files.

# git

for commits use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) format:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Where type is one of:

* feat: a commit of the type feat introduces a new feature to the codebase (this correlates with MINOR in Semantic Versioning).
* fix: a commit of the type fix patches a bug in your codebase (this correlates with PATCH in Semantic Versioning).
* test: for adding missing tests or correcting existing tests
* ci: for all CI related changes.
* build: for all build related changes, like bazel BUILD files, or changes to the bazel build system.
* docs: for documentation changes
* refactor: for code refactoring that doesn't change functionality
* style: for code style changes (white-space, formatting, missing semi-colons, etc) that do not affect the meaning of the code
* perf: for code changes that improve performance
* chore: for all other changes that don't fit into the above categories, like updating dependencies, or other maintenance tasks

Do not use scopes like feat(ci) or fix(ci). Use the dedicated `ci:` and `build:` prefixes instead. Use scopes for other types if needed, like feat(api) or fix(ui).
