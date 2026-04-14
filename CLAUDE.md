This is a monorepo with bazel. Use `bazel run` and `bazel build` and `bazel test`, instead of `go test`.

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

feat, fix, test, refactor, docs MUST include the scope, if applicable. For example, 
* If you are adding a new feature to bifrost, the commit message should be `feat(bifrost): add new feature`. 
* If you are fixing a bug in knife, the commit message should be `fix(knife): fix bug`.
* If you are adding a new test for a feature in resolve-secrets, the commit message should be `test(resolve-secrets): add new test`.


## Review

Google's code review guidelines are public at https://google.github.io/eng-practices/review/. The core principles:

The Standard: Approve when the CL improves overall code health, even if not perfect. Don't block on perfection — block on correctness, clarity, and maintainability.

What to look for:

1. Design — Is this the right abstraction? Does it belong here? Does it integrate well with the rest of the system?
2. Functionality — Does it do what the author intended? Are there edge cases? Is it good for users (both end-users and future developers)?
3. Complexity — Can it be understood quickly? Will it introduce bugs when someone modifies it later? Over-engineering counts as complexity.
4. Tests — Correct, sensible, useful tests. Will they fail when the code is broken? Will they produce false positives?
5. Naming — Clear enough to communicate what it is/does without being too long?
6. Comments — Explain why, not what. If the code needs a comment to explain what it does, simplify the code instead.
7. Style — Consistent with the codebase.
8. Every line — You're expected to read every line you're assigned. Look at context, not just the diff.

Key review attitudes:
- Be kind and constructive
- Label severity: "nit:", "optional:", "consider:" vs blocking comments
- Explain why — don't just say "change this"
- Compliment good patterns when you see them
