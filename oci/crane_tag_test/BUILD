load("@bazel_skylib//:bzl_library.bzl", "bzl_library")
load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@rules_shell//shell:sh_test.bzl", "sh_test")
load("//oci:crane_tag.bzl", "crane_tag")
load(":fixture.bzl", "digest_fixture")

# A stand-in for a rules_img image: anything exposing a `digest` output
# group, which is all `crane_tag` reads.
digest_fixture(
    name = "image",
    digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
)

# Literal tags plus one `{{.STABLE_*}}` placeholder. The placeholder is
# resolved from the workspace status file at build time; the test below
# supplies its own status so the expansion is byte-stable.
crane_tag(
    name = "image_tag",
    image = ":image",
    repository = "ghcr.io/example/mirror/app",
    tag_list = [
        "latest",
        "{{.STABLE_TEST_VERSION}}",
        "{{.STABLE_TEST_VERSION}}-debug",
        # Every character a sed replacement would mangle. Not a valid OCI
        # tag; the assertion is that expansion is literal.
        "{{.STABLE_TEST_SPECIALS}}",
    ],
    workspace_status = ":status.txt",
)

# The expanded tag list is a build output, so it can be diffed without a
# registry in sight.
diff_test(
    name = "tags_expanded_test",
    file1 = ":image_tag.tags",
    file2 = "expected_tags.txt",
)

# Running the target with a fake crane (via the CRANE override) records
# exactly the commands it would issue: one `tag` per entry, all against the
# digest, never a tag as the source.
sh_test(
    name = "commands_test",
    srcs = ["commands_test.sh"],
    data = [
        "expected_commands.txt",
        ":image_tag",
    ],
    env = {
        "TAG_BIN": "$(rootpath :image_tag)",
        "EXPECTED": "$(rootpath expected_commands.txt)",
    },
)

bzl_library(
    name = "fixture",
    srcs = ["fixture.bzl"],
    visibility = ["//visibility:public"],
)
