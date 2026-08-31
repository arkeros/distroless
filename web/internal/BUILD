# The Go toolchain enforces `internal/` in `go/build`, which rules_go does not
# consult — under Bazel the directory name alone restricts nothing. Bazel
# visibility is the mechanism that holds, so declare it here and let it apply
# to every package beneath: nothing outside //web may depend on these.
#
# gazelle:default_visibility //web:__subpackages__

package(default_visibility = ["//web:__subpackages__"])
