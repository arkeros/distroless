"""Re-export for `load("@terraform.bzl", ...)` syntax sugar."""

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
tf_script_test = _tf_script_test
tf_script_binary = _tf_script_binary
tf_toolchain = _tf_toolchain
