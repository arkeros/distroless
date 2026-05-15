"""Top-level load surface for rules_rpm."""

load("//rpm:defs.bzl", _rpm_package = "rpm_package", _rpmdb_merge = "rpmdb_merge")

rpm_package = _rpm_package
rpmdb_merge = _rpmdb_merge
