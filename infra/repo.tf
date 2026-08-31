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

# Two rulesets, because bypass in GitHub is per-ruleset and not per-rule.
#
# Folding these into one would mean the admin bypass that exists for the review
# requirement also switched off the status check — and the status check is the
# entire reason the deploy can run unattended. Splitting them keeps the human
# step optional for an admin while leaving "what is on `main` compiled and
# passed its tests" true for everyone, admins included.

# No bypass_actors. This is the invariant the deploy rests on.
resource "github_repository_ruleset" "main_checks" {
  name        = "main-checks"
  repository  = local.github_repo_name
  target      = "branch"
  enforcement = "active"

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
      # `Test` is the `name:` of the only ci.yaml job that runs on
      # `pull_request`; the rest are `if: github.event_name == 'push'`.
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
      required_approving_review_count = 1

      # Carried over from the classic branch protection this replaces.
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

  allow_squash_merge     = true
  allow_merge_commit     = true
  allow_rebase_merge     = true
  allow_update_branch    = false
  delete_branch_on_merge = false

  squash_merge_commit_title   = "COMMIT_OR_PR_TITLE"
  squash_merge_commit_message = "COMMIT_MESSAGES"
  merge_commit_title          = "MERGE_MESSAGE"
  merge_commit_message        = "PR_TITLE"

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
