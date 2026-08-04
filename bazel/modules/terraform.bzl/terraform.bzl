"""Re-export shim for `load("@terraform.bzl", ...)` syntax sugar.

Core Starlark surface: `tf_root` + the resource/output/var helpers it
composes with, plus the toolchain and lint rules. Resource constructors
for specific terraform providers live in sibling shims to keep imports
namespaced — see `@terraform.bzl//:gcp.bzl` and `@terraform.bzl//:k8s.bzl`.
"""

load(
    "//terraform:deploy.bzl",
    _TfDeployInfo = "TfDeployInfo",
    _deploy_task = "deploy_task",
)
load(
    "//terraform:refs.bzl",
    _TfExportsInfo = "TfExportsInfo",
    _ref = "ref",
)
load(
    "//terraform:defs.bzl",
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

# Deploy-DAG vocabulary, for nodes that are not Terraform roots.
# `deploy_task` makes an existing runnable a node; `TfDeployInfo` is what
# `deploy_after` requires, so it is also what a rule outside this module would
# provide to become a node in its own right.
deploy_task = _deploy_task
TfDeployInfo = _TfDeployInfo

# Cross-root references. `ref` names another root's `exports` entry; the deploy
# edge comes from the same token, so the two cannot disagree.
ref = _ref
TfExportsInfo = _TfExportsInfo
tf_script_test = _tf_script_test
tf_script_binary = _tf_script_binary
tf_toolchain = _tf_toolchain
