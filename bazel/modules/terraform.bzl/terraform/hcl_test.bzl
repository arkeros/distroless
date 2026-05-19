"""Unit tests for `parse_terraform_lockfile`.

skylib's `unittest` framework runs the test functions at analysis time;
they exercise the parser directly with synthetic lockfile bodies and
assert on the returned dict. `fail()` paths aren't covered here — they
need an `analysistest` setup, which is heavier and overkill for a
parser whose error path is "user has a malformed lockfile, sees an
error with file+line."
"""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":hcl.bzl", "parse_terraform_lockfile")

# ---------- happy paths -----------------------------------------------------

def _empty_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, {}, parse_terraform_lockfile(""))
    asserts.equals(env, {}, parse_terraform_lockfile("\n\n"))
    return unittest.end(env)

empty_test = unittest.make(_empty_impl)

def _comments_only_impl(ctx):
    env = unittest.begin(ctx)
    body = """
# This file is maintained automatically by "terraform init".
# Manual edits may be lost in future updates.
"""
    asserts.equals(env, {}, parse_terraform_lockfile(body))
    return unittest.end(env)

comments_only_test = unittest.make(_comments_only_impl)

def _single_provider_impl(ctx):
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/google" {
  version     = "7.32.0"
  constraints = "~> 7.0"
  hashes = [
    "h1:abc=",
    "zh:def",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, ["hashicorp/google"], got.keys())
    asserts.equals(env, "7.32.0", got["hashicorp/google"]["version"])
    asserts.equals(env, "~> 7.0", got["hashicorp/google"]["constraints"])
    asserts.equals(env, ["h1:abc=", "zh:def"], got["hashicorp/google"]["hashes"])
    return unittest.end(env)

single_provider_test = unittest.make(_single_provider_impl)

def _multiple_providers_impl(ctx):
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/google" {
  version = "7.32.0"
  hashes = [
    "h1:g=",
  ]
}

provider "registry.terraform.io/hashicorp/kubernetes" {
  version = "2.38.0"
  hashes = [
    "h1:k=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, sorted(["hashicorp/google", "hashicorp/kubernetes"]), sorted(got.keys()))
    asserts.equals(env, "7.32.0", got["hashicorp/google"]["version"])
    asserts.equals(env, "2.38.0", got["hashicorp/kubernetes"]["version"])
    return unittest.end(env)

multiple_providers_test = unittest.make(_multiple_providers_impl)

def _trailing_comma_impl(ctx):
    # Hashes with and without trailing commas should both parse.
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/foo/bar" {
  version = "1.0.0"
  hashes = [
    "h1:trailing=",
    "h1:no-trailing="
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, ["h1:trailing=", "h1:no-trailing="], got["foo/bar"]["hashes"])
    return unittest.end(env)

trailing_comma_test = unittest.make(_trailing_comma_impl)

def _registry_prefix_stripped_impl(ctx):
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/random" {
  version = "3.9.0"
  hashes = [
    "h1:r=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, ["hashicorp/random"], got.keys())
    return unittest.end(env)

registry_prefix_stripped_test = unittest.make(_registry_prefix_stripped_impl)

def _non_registry_address_preserved_impl(ctx):
    # A non-registry.terraform.io address shouldn't have anything stripped.
    env = unittest.begin(ctx)
    body = """
provider "example.com/acme/widget" {
  version = "1.0.0"
  hashes = [
    "h1:w=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, ["example.com/acme/widget"], got.keys())
    return unittest.end(env)

non_registry_address_preserved_test = unittest.make(_non_registry_address_preserved_impl)

def _constraints_optional_impl(ctx):
    # `constraints =` is not always present (terraform omits it when no
    # version constraint was declared); the parser must accept that.
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/foo" {
  version = "1.0.0"
  hashes = [
    "h1:f=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, "1.0.0", got["hashicorp/foo"]["version"])
    asserts.true(env, "constraints" not in got["hashicorp/foo"])
    return unittest.end(env)

constraints_optional_test = unittest.make(_constraints_optional_impl)

def _comments_inside_block_impl(ctx):
    # Stray `#` lines should be skipped wherever they appear.
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/foo" {
  # explanatory line
  version = "1.0.0"
  hashes = [
    # another comment
    "h1:f=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, "1.0.0", got["hashicorp/foo"]["version"])
    asserts.equals(env, ["h1:f="], got["hashicorp/foo"]["hashes"])
    return unittest.end(env)

comments_inside_block_test = unittest.make(_comments_inside_block_impl)

def _multispace_alignment_impl(ctx):
    # Terraform aligns `version` / `constraints` etc. with multiple spaces
    # before the `=`. The parser must accept arbitrary whitespace.
    env = unittest.begin(ctx)
    body = """
provider "registry.terraform.io/hashicorp/foo" {
  version     = "1.0.0"
  constraints = "~> 1.0"
  hashes = [
    "h1:f=",
  ]
}
"""
    got = parse_terraform_lockfile(body)
    asserts.equals(env, "1.0.0", got["hashicorp/foo"]["version"])
    asserts.equals(env, "~> 1.0", got["hashicorp/foo"]["constraints"])
    return unittest.end(env)

multispace_alignment_test = unittest.make(_multispace_alignment_impl)

def hcl_test_suite(name):
    """Wire all `parse_terraform_lockfile` tests under one suite target."""
    unittest.suite(
        name,
        empty_test,
        comments_only_test,
        single_provider_test,
        multiple_providers_test,
        trailing_comma_test,
        registry_prefix_stripped_test,
        non_registry_address_preserved_test,
        constraints_optional_test,
        comments_inside_block_test,
        multispace_alignment_test,
    )
