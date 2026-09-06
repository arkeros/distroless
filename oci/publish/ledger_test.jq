# Runs every case in ledger_test_cases.json through moves() and lists the ones
# whose answer is not the one wanted. Empty output is a pass; the diff_test in
# BUILD holds it to that. A rule that fails is the answer "error", so a case
# can ask for that too.
include "ledger";

def answer:
  . as $case
  | .ledger
  | try moves($case.applied; $case.at; $case.run) catch "error";

map(
  answer as $got
  | select($got != .want)
  | {name, want, got: $got}
)
