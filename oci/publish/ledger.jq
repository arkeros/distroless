# Which tag moves the release CI just made are new to a family's tag ledger.
# See ADR 0017.
#
# Entry point: `moves($applied; $at; $run)`, with the ledger's existing events
# (one JSON object per line, slurped) as the input array, the `{tag, digest}`
# pairs the release applied as $applied, the time of the release as $at, and
# the URL of the run as $run. Yields the events to append: one per applied
# tag whose last recorded digest is not the one just applied. Fails when the
# ledger or the applied list is not what it claims to be. The caller must stop
# on a failure, not guess: a rule that answers "nothing moved" to a ledger it
# cannot read loses the history silently, and one that answers "everything
# moved" fills it with events that are not moves.
#
# From CI, with the module on the search path:
#
#   jq -c -s -L oci/publish --argjson applied "$APPLIED" --arg at "$AT" --arg run "$RUN" \
#       'include "ledger"; moves($applied; $at; $run)[]' ledger.jsonl

# A binding is a tag and the build it names. An event is a binding with when
# it was made; `run` is where, kept so a reader can get from the event to the
# job, and not validated because nothing here decides on it.
def binding_ok:
  type == "object"
  and (.tag | type == "string")
  and (.digest | type == "string" and startswith("sha256:"));

def event_ok:
  binding_ok and (.at | type == "string");

# The last event for each tag, by file order: the ledger is append-only, so
# the last line about a tag is the current binding. Not by `at`, which two
# releases in one second could share.
def current:
  reduce .[] as $event ({}; .[$event.tag] = $event.digest);

def moves($applied; $at; $run):
  if all(event_ok) | not then
    error("not a tag ledger: every event needs at, tag and a sha256: digest")
  elif ($applied | type != "array" or (all(binding_ok) | not)) then
    error("not a list of applied tags: every entry needs tag and a sha256: digest")
  elif ($applied | map(.tag) | length) != ($applied | map(.tag) | unique | length) then
    error("a tag was applied twice in one release")
  else
    current as $current
    | $applied
    | map(select($current[.tag] != .digest) | {$at, tag, digest, $run})
  end;
