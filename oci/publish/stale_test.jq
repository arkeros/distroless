# Runs every case in stale_test_cases.json through stale() and lists the ones
# whose verdict is not the one wanted. Empty output is a pass; the diff_test
# in BUILD holds it to that. A rule that fails is the verdict "error", so a
# case can ask for that too.
include "stale";

def verdict:
  . as $case
  | .attached
  | try stale($case.type; $case.local) catch "error";

map(
  verdict as $got
  | select($got != .want)
  | {name, want, got: $got}
)
