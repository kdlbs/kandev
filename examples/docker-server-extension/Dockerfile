# Optional PostgreSQL 15 client for local/worktree agents in the server image.
# Update the Kandev release tag, digest, and Debian package version together.
FROM ghcr.io/kdlbs/kandev:0.98.0@sha256:f9aab231d3b3d3c09b0cfd247c8aa007bac2bc0788d1ae776af79c5307bbf628

USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends postgresql-client-15=15.19-0+deb12u1 \
 && psql --version \
 && rm -rf /var/lib/apt/lists/*

# Keep the base image's entrypoint and root startup: it repairs /data ownership,
# then uses gosu to launch Kandev and local/worktree agents as user kandev.
