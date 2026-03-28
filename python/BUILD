load("@pip//:requirements.bzl", "all_whl_requirements")
load("@rules_python_gazelle_plugin//manifest:defs.bzl", "gazelle_python_manifest")
load("@rules_python_gazelle_plugin//modules_mapping:def.bzl", "modules_mapping")
load("@rules_uv//uv:pip.bzl", "pip_compile")
load("@rules_uv//uv:venv.bzl", "create_venv")

pip_compile(
    name = "generate_requirements_linux_txt",
    python_platform = "x86_64-unknown-linux-gnu",
    requirements_in = ":requirements.in",
    requirements_txt = "requirements_linux.txt",
)

pip_compile(
    name = "generate_requirements_macos_txt",
    python_platform = "aarch64-apple-darwin",
    requirements_in = ":requirements.in",
    requirements_txt = "requirements_macos.txt",
)

create_venv(
    name = "create_venv",
    requirements_txt = select({
        "@platforms//os:linux": ":requirements_linux.txt",
        "@platforms//os:osx": ":requirements_macos.txt",
    }),
)

modules_mapping(
    name = "modules_map",
    wheels = all_whl_requirements,
)

gazelle_python_manifest(
    name = "gazelle_python_manifest",
    modules_mapping = ":modules_map",
    pip_repository_name = "pip",
    requirements = ":requirements_linux.txt",
)
