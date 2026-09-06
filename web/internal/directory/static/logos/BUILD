package(default_visibility = ["//web:__subpackages__"])

# The family marks the directory's front page inlines, one file per family,
# globbed so adding a family's logo needs no BUILD edit beyond this package.
filegroup(
    name = "logos",
    srcs = glob(["*.svg"]),
    visibility = ["//web/internal/directory:__pkg__"],
)
