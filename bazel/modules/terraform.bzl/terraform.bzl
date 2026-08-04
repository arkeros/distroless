"""Re-export shim for `load("@terraform.bzl", ...)` syntax sugar.

Core Starlark surface: `tf_root` + the resource/output/var helpers it
composes with, plus the toolchain and lint rules. Resource constructors
for specific terraform providers live in sibling shims to keep imports
namespaced — see `@terraform.bzl//:gcp.bzl` and `@terraform.bzl//:k8s.bzl`.
"""

load(
    "//terraform:defs.bzl",
    _DEPLOY_TAG_KIND_TASK = "DEPLOY_TAG_KIND_TASK",
    _deploy_tags = "deploy_tags",
    _deploy_task = "deploy_task",
    _merge_tf = "merge_tf",
    _output = "output",
    _remote_state = "remote_state",
    _resource = "resource",
    _tf_root = "tf_root",
    _var = "var",
    _variable = "variable",
)
load(
    "//terraform:lint.bzl",
    _tf_script_binary = "tf_script_binary",
    _tf_script_test = "tf_script_test",
)
load(
    "//terraform/toolchain:toolchain.bzl",
    _tf_toolchain = "tf_toolchain",
)

tf_root = _tf_root
resource = _resource
output = _output
remote_state = _remote_state
var = _var
variable = _variable
merge_tf = _merge_tf

# Deploy-DAG helpers, for nodes that are not Terraform roots.
# `deploy_task` wraps an existing runnable; `deploy_tags` is the primitive
# underneath it, for macros that create their own target and can tag it
# directly.
deploy_task = _deploy_task
deploy_tags = _deploy_tags
DEPLOY_TAG_KIND_TASK = _DEPLOY_TAG_KIND_TASK
tf_script_test = _tf_script_test
tf_script_binary = _tf_script_binary
tf_toolchain = _tf_toolchain
