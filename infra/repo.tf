# GitHub repository settings that the deploy's security model depends on.
#
# The WIF binding in github.tf is keyed on `attribute.environment/prod`, and
# GitHub only emits that claim after validating the `prod` environment's rules.
# That makes those rules part of the identity gate rather than repo cosmetics —
# so they belong in Terraform next to the binding that trusts them, not in a
# settings page whose contents no reviewer ever sees.
#
# The provider authenticates by falling back to `gh auth token`, so applying
# this needs no PAT beyond the `gh` login the operator already has.

locals {
  github_owner     = split("/", local.github_repository)[0]
  github_repo_name = split("/", local.github_repository)[1]
}

# The environment predates this file; adopt it rather than fail on create.
# Once applied this block is a no-op and can be deleted, but leaving it means
# a rebuilt state file adopts instead of colliding.
import {
  to = github_repository_environment.prod
  id = "${local.github_repo_name}:prod"
}

# What gates the OIDC claim is the branch policy, not a human. A job declaring
# `environment: prod` on a branch this policy excludes never starts, so no
# token carrying `environment: prod` is ever minted and the deploy service
# account is unreachable. Reviewers were only ever a second, weaker layer on
# top of that: the sole reviewer was also the only person who could push to
# `main`, self-review was permitted, and admins could bypass — so the prompt
# cost a round-trip per deploy and denied nothing. `main` is now guarded by the
# rulesets below, which is where "this code is fit to deploy" is actually
# decided, so the deploy itself runs unattended.
resource "github_repository_environment" "prod" {
  repository  = local.github_repo_name
  environment = "prod"

  # Deliberately no `reviewers` block — see above.

  # `false` is the conservative reading: admin bypass is documented against the
  # protection rules, not the branch policy, but this environment's whole value
  # is that the branch policy holds, so do not leave that to interpretation.
  can_admins_bypass = false

  # Naming `main` explicitly, rather than `protected_branches = true`. The
  # latter admits *any* branch that happens to carry a protection rule, which
  # made the "main-only" claim in github.tf and ci.yaml true by accident.
  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "prod_main" {
  repository     = local.github_repo_name
  environment    = github_repository_environment.prod.environment
  branch_pattern = "main"
}

# Two rulesets, because bypass in GitHub is per-ruleset and not per-rule, so
# each can be relaxed for admins without the other following.

# Repository admins may bypass. This was set on the live ruleset and is kept:
# with a single maintainer, an emergency push to `main` that cannot wait for
# a green pull-request check has to be possible, and the alternative is
# disabling the ruleset.
#
# That is safe because this rule is not what keeps untested code out of
# production. ci.yaml runs on every push to `main` and its `test` job reruns
# `bazel test //...` there; `publish` needs it, and the deploy needs `publish`.
# A bypass skips the *pre-merge* check on the pull request, and the push-to-
# `main` run then fails on the same red test before anything is published.
# What the bypass costs is that `main` can be red until the next commit, not
# that a red commit ships. Drop the block when a second maintainer exists.
resource "github_repository_ruleset" "main_checks" {
  name        = "main-checks"
  repository  = local.github_repo_name
  target      = "branch"
  enforcement = "active"

  bypass_actors {
    actor_id    = 5 # base repository role: admin
    actor_type  = "RepositoryRole"
    bypass_mode = "always"
  }

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    # Both read as "only someone with bypass may do this", and nobody has
    # bypass here — so: no force-pushes, and `main` cannot be deleted.
    non_fast_forward        = true
    deletion                = true
    required_linear_history = true

    required_status_checks {
      # `Test` is the `name:` of the one job in pr.yaml, the only workflow
      # that runs on `pull_request`; ci.yaml runs on push to `main` alone.
      # Renaming that job silently disables this rule, so the two names have
      # to move together.
      required_check {
        context = "Test"
      }

      # Strict: a branch has to be current with `main` before it can merge,
      # so `Test` is green on the tree that actually lands and not merely on
      # the one the author started from.
      #
      # The push-to-`main` run would catch a bad merge of two individually
      # green pull requests regardless — `deploy` needs `publish` needs `test`,
      # so nothing untested reaches Artifact Registry either way. The
      # difference is where it is caught: without this, `main` is already red
      # and the fix is a follow-up commit.
      #
      # The cost is a rebase-and-rerun on pull requests that fall behind, which
      # Renovate absorbs by itself. Its `rebaseWhen` default of `auto` resolves
      # to `behind-base-branch` when the platform requires up-to-date branches,
      # and it inspects rulesets — not just the legacy branch protection — to
      # decide that. So every Renovate pull request stays rebased, not only the
      # minor and patch ones //.github/renovate.json automerges.
      strict_required_status_checks_policy = true
    }
  }
}

# Bypassable by repository admins. The review requirement is a practice worth
# encoding for when this repo has more than one maintainer; today the sole
# maintainer cannot approve their own pull request, so requiring an approval
# with no bypass would mean either never merging or disabling the rule.
resource "github_repository_ruleset" "main_review" {
  name        = "main-review"
  repository  = local.github_repo_name
  target      = "branch"
  enforcement = "active"

  # Inert today, and kept for when it is not. Repository roles are assigned to
  # collaborators, and on a user-owned repository the owner holds implicit
  # ownership rather than an assigned role — so there is nothing here for this
  # to match, and GitHub reports `viewerCanMergeAsAdmin: false` for the owner.
  # It starts working the moment a second admin collaborator exists.
  bypass_actors {
    actor_id    = 5 # base repository role: admin
    actor_type  = "RepositoryRole"
    bypass_mode = "always"
  }

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
    pull_request {
      # `main-checks` requires linear history, so a merge commit is rejected
      # after the fact. Naming the two that can succeed turns that into an
      # option GitHub never offers, rather than a button that fails. The
      # repository's own `allow_merge_commit` is false for the same reason —
      # this enforces it, that hides it.
      allowed_merge_methods = ["squash", "rebase"]

      # Zero, not one, while this repository has a single maintainer.
      #
      # One approval is unsatisfiable here: nobody else can give it, and the
      # bypass above does not reach the owner for the reason noted on it. The
      # rule would therefore be overridden on every merge, and a rule that is
      # always overridden is worse than no rule — it trains the reflex that
      # would one day skip `main-checks`, which deliberately has no bypass at
      # all. `arkeros/senku` sits at zero for the same reason.
      #
      # Raise this to 1 when a second maintainer joins; nothing else has to
      # change, since the bypass starts applying to admins at the same moment.
      required_approving_review_count = 0

      # Carried over from the classic branch protection this replaces. Still
      # satisfiable by one person: you can resolve your own threads.
      required_review_thread_resolution = true
    }
  }
}

# The repository itself, adopted for the sake of one setting: `allow_auto_merge`.
#
# Renovate's `platformAutomerge` in //.github/renovate.json is a no-op unless
# GitHub's auto-merge is enabled, so the two have to agree — and until this
# resource existed, the half that mattered lived only in a settings page. That
# is the same drift the rulesets above were pulled into Terraform to kill.
#
# Adopting a whole `github_repository` to hold one boolean is a real cost: every
# other attribute here is transcribed from the live repository, and any one this
# omits would be reset to the provider's default on the next apply. They are
# spelled out rather than left implicit for that reason.
#
# Destroying this resource deletes the repository, so it carries two guards
# that fail in different situations. `prevent_destroy` refuses a destroy while
# the block is in the configuration — but it is part of that block, so it is
# gone the moment the resource is not, which is exactly what a plan run from a
# checkout predating this file does: it reports the repository as "not in
# configuration" and proposes deleting it. `archive_on_destroy` lives in state
# rather than in config, so it still applies then, and turns the irreversible
# outcome into an archived repository that can be restored.
resource "github_repository" "this" {
  name         = local.github_repo_name
  description  = "Minimal, reproducible distroless OCI base images built with Bazel — signed, SBOM'd and CVE-scanned."
  homepage_url = "distroless.io"
  visibility   = "public"

  has_issues   = true
  has_projects = true
  has_wiki     = true
  is_template  = false
  archived     = false
  topics       = []

  web_commit_signoff_required = false

  # The one that is actually load-bearing. Everything else in this block is
  # transcription.
  allow_auto_merge = true

  allow_squash_merge = true
  allow_rebase_merge = true

  # False because `main-checks` requires linear history: a merge commit cannot
  # land on `main`, so offering the button only produces a failed merge.
  allow_merge_commit = false

  allow_update_branch = false

  # Set in the repository settings and kept: the branch a squash-merged pull
  # request came from has nothing left to say once it lands.
  delete_branch_on_merge = true

  squash_merge_commit_title   = "COMMIT_OR_PR_TITLE"
  squash_merge_commit_message = "COMMIT_MESSAGES"

  # Inert while `allow_merge_commit` is false, and still transcribed: dropping
  # them would reset them to the provider's defaults for no reason, and they
  # are what GitHub would use if merge commits were ever re-enabled.
  merge_commit_title   = "MERGE_MESSAGE"
  merge_commit_message = "PR_TITLE"

  # Survives the resource leaving the configuration; `prevent_destroy` does not.
  archive_on_destroy = true

  lifecycle {
    prevent_destroy = true
  }
}

import {
  to = github_repository.this
  id = local.github_repo_name
}

# Its own resource rather than the `vulnerability_alerts` attribute, which the
# provider deprecates in favour of this.
resource "github_repository_vulnerability_alerts" "this" {
  repository = github_repository.this.name
  enabled    = true
}

import {
  to = github_repository_vulnerability_alerts.this
  id = local.github_repo_name
}
