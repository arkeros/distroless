load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "terraform",
    srcs = ["terraform.go"],
    importpath = "github.com/arkeros/senku/devtools/bifrost/terraform",
    visibility = ["//visibility:public"],
    deps = [
        "//devtools/bifrost/api",
        "@com_github_hashicorp_hcl_v2//:hcl",
        "@com_github_hashicorp_hcl_v2//hclwrite",
        "@com_github_zclconf_go_cty//cty",
    ],
)

go_test(
    name = "terraform_test",
    srcs = ["terraform_test.go"],
    data = glob(["testdata/**"]),
    embed = [":terraform"],
    deps = [
        "//devtools/bifrost/testing",
        "//testing/golden",
    ],
)
