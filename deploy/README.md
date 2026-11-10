# Deploy

Target: Scaleway Kapsule (Paris), namespaces `staging` and `prod`. EU-only infrastructure.

To come (issue #3): Dockerfiles for api, worker, mailin and web, Kubernetes manifests, and a deploy script.

DNS notes (issue #5): `laamo.app` (site + app), `notify.laamo.app` (Lane 1), `in.laamo.app` (inbound MX), separate reply domain (Lane 2, name TBD).
