load("@gazelle//:def.bzl", "DEFAULT_LANGUAGES", "gazelle", "gazelle_binary", "gazelle_test")

exports_files([
    # Consumed by rules_rpm via @hummingbird//... — see bazel/include/oci.MODULE.bazel.
    "hummingbird-release.pgp",
    "hummingbird.lock.json",
    "hummingbird_install.json",
])

# Ignore Claude Code worktrees
# gazelle:exclude .claude
# Prefer generated BUILD files to be called BUILD over BUILD.bazel
# gazelle:build_file_name BUILD,BUILD.bazel
# gazelle:prefix github.com/arkeros/senku
# Use new style for go_naming_convention https://github.com/bazelbuild/bazel-gazelle/issues/5#issuecomment-636056748
# gazelle:go_naming_convention import
#
# Make these the default compilers for proto rules.
# See https://github.com/bazelbuild/rules_go/pull/3761 for more details
# gazelle:go_grpc_compilers	@rules_go//proto:go_proto,@rules_go//proto:go_grpc_v2
#
# Copied from https://github.com/grpc-ecosystem/grpc-gateway/blob/bffd8c6998e483da652c18b56c4f9d6f27d6304f/BUILD.bazel#L22
#
# gazelle:resolve proto proto google/api/annotations.proto @googleapis//google/api:annotations_proto
# gazelle:resolve proto go google/api/annotations.proto  @org_golang_google_genproto_googleapis_api//annotations
# gazelle:resolve proto proto google/api/http.proto @googleapis//google/api:http_proto
# gazelle:resolve proto go google/api/http.proto  @org_golang_google_genproto_googleapis_api//annotations
# gazelle:resolve proto proto google/api/field_behavior.proto @googleapis//google/api:field_behavior_proto
# gazelle:resolve proto go google/api/field_behavior.proto  @org_golang_google_genproto_googleapis_api//annotations
# gazelle:resolve proto proto google/api/client.proto @googleapis//google/api:client_proto
# gazelle:resolve proto go google/api/client.proto  @org_golang_google_genproto_googleapis_api//annotations
# gazelle:resolve proto proto google/api/httpbody.proto @googleapis//google/api:httpbody_proto
# gazelle:resolve proto go google/api/httpbody.proto  @org_golang_google_genproto_googleapis_api//httpbody
# gazelle:resolve proto proto google/api/visibility.proto @googleapis//google/api:visibility_proto
# gazelle:resolve proto go google/api/visibility.proto  @org_golang_google_genproto_googleapis_api//visibility
# gazelle:resolve proto proto google/api/resource.proto @googleapis//google/api:resource_proto
# gazelle:resolve proto go google/api/resource.proto  @org_golang_google_genproto_googleapis_api//annotations
# gazelle:resolve proto proto google/rpc/status.proto @googleapis//google/rpc:status_proto
# gazelle:resolve proto go google/rpc/status.proto  @org_golang_google_genproto_googleapis_rpc//status
# gazelle:resolve proto proto buf/validate/validate.proto @protovalidate//proto/protovalidate/buf/validate:validate_proto
# gazelle:resolve proto go buf/validate/validate.proto @build_buf_gen_go_bufbuild_protovalidate_protocolbuffers_go//buf/validate
# gazelle:resolve go google.golang.org/grpc/health/grpc_health_v1 @org_golang_google_grpc//health/grpc_health_v1
#
# Both `cyclonedx.bzl` and `sbom.bzl` ship under a single `sbom` bzl_library
# upstream; map gazelle's per-file import lookup to the same combined label.
# gazelle:resolve bzl @supply_chain_tools//sbom:cyclonedx.bzl @supply_chain_tools//sbom
# gazelle:resolve bzl @supply_chain_tools//sbom:sbom.bzl @supply_chain_tools//sbom

gazelle_binary(
    name = "gazelle_bin",
    languages = DEFAULT_LANGUAGES + [
        "@bazel_skylib_gazelle_plugin//bzl",
        # "@rules_buf//gazelle/buf:buf",
    ],
)

gazelle(
    name = "gazelle",
    gazelle = ":gazelle_bin",
)

gazelle_test(
    name = "gazelle.check",
    size = "small",
    gazelle = ":gazelle_bin",
    workspace = "//:BUILD",
)
