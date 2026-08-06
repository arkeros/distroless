"""Bifrost macro for a static site served from a GCS bucket behind Cloud CDN.

`site_gcs(...)` — a bucket holding a built webroot, plus the public-read
binding a backend bucket needs. Returns one struct shaped for
`tf_root(docs=...)`, with a `bucket_name` field the LB root uses to build its
`google_compute_backend_bucket`.

This is the counterpart to `service_cloudrun` for frontends that have no
server. A Cloud Run origin runs the app's image and answers requests from it;
a bucket never runs anything, which is the whole point — there is no instance
to start, so there is no first request that pays for starting one.

What a bucket cannot do is decide anything per request. Cache headers are
metadata written onto each object by `bucket_push`, and the SPA's
history-API fallback belongs to the URL map, not here — see
`//infra/cloud/gcp/lb:defs.bzl`.
"""

load(
    "@terraform.bzl//:gcp.bzl",
    "storage_bucket",
    "storage_bucket_iam_member",
)
load("@terraform.bzl", "merge_tf")

def site_gcs(
        name,
        project,
        bucket_name,
        location,
        labels = None):
    """A GCS bucket holding a static site, readable by the LB.

    Args:
        name: Terraform resource block key; the IAM binding is derived from
            it.
        project: GCP project.
        bucket_name: The bucket's global name. Callers derive it from the
            project and service name rather than letting this macro invent
            one, because `bucket_push` has to name the same bucket without
            resolving Terraform state.
        location: Bucket location. A single region close to the CDN edge is
            right for an origin — the CDN is what makes the site global, and
            a multi-region bucket would pay for replication that only the
            cache-miss path ever reads.
        labels: Optional bucket labels.

    Returns:
        A struct with `tf` (the merged documents), `bucket_name` (a literal,
        for `bucket_push` and the LB's backend bucket) and `url`.
    """
    bucket = storage_bucket(
        name = name,
        project = project,
        bucket_name = bucket_name,
        location = location,
        # No per-object ACLs: every object in a webroot has exactly the same
        # audience, so the one grant below says all of it.
        uniform_bucket_level_access = True,
        # The bucket holds only build output. Anything lost by destroying it
        # is reproduced by the next `bucket_push`, so requiring the bucket to
        # be emptied by hand first would protect nothing.
        force_destroy = True,
        # `inherited` rather than `enforced`: the grant below is public by
        # necessity, not by oversight. See its comment.
        public_access_prevention = "inherited",
        # This is the deletion half of a publish. `bucket_push` does not
        # delete the objects a build stops producing — a browser that loaded
        # the previous index.html still fetches a lazy route's chunk when the
        # user navigates, which can be long after the deploy landed, and
        # deleting on the publish that orphaned it turns that navigation into
        # a 404. The publish stamps `customTime` on an orphan instead and
        # this rule collects it 30 days later, which outlasts any session.
        #
        # Retention belongs here rather than in the uploader for two reasons:
        # ADR 0009 gives Terraform the bucket's lifecycle, and a rule keeps
        # collecting whether or not anyone ever deploys again.
        #
        # A re-uploaded object is a new generation with no `customTime`, so a
        # hash that comes back stops matching this condition on its own.
        lifecycle_rule = [{
            "condition": [{"days_since_custom_time": 30}],
            "action": [{"type": "Delete"}],
        }],
        # Without this, a request for the bucket root returns an XML listing
        # of every object — with a 200. That is not a 404, so the URL map's
        # SPA fallback never sees one and never fires, and `/` serves XML to
        # a browser that renders nothing. `/index.html` and unknown routes
        # both work in that state, which is what makes it easy to miss: the
        # one broken URL is the only one anybody actually visits.
        #
        # No `not_found_page`. It would serve index.html for a missing
        # object and keep the 404 — which is now exactly what the URL map's
        # fallback does, so this would be a second mechanism doing one
        # thing. The fallback is the one that stays, because it is also
        # where a declared route's 200 is reasoned about. Root pages here,
        # unknown routes there, one mechanism each. See
        # //infra/cloud/gcp/lb:defs.bzl and ADR 0011.
        website = {"main_page_suffix": "index.html"},
        labels = labels,
    )

    # `google_compute_backend_bucket` reads the bucket anonymously — it has
    # no service identity to present — so without this the LB gets 403 on
    # every object and the site is unreachable rather than merely uncached.
    #
    # The objects are a public website's assets: they are served to any
    # browser that loads the page regardless. The grant widens who can
    # read them by bucket URL as well as by site URL, which for compiled
    # frontend bytes is not a distinction worth defending.
    public = storage_bucket_iam_member(
        name = name + "_public",
        bucket = bucket.name,
        role = "roles/storage.objectViewer",
        member = "allUsers",
    )

    return struct(
        tf = merge_tf(bucket, public),
        bucket_name = bucket_name,
        url = bucket.url,
    )
