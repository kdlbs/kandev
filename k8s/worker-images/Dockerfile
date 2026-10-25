# Kandev worker image recipes.
#
# These images are task-session inputs, not control-plane images. Kandev still
# owns the command, helper, credentials, runtime volume, and workspace mount.

ARG BASE_IMAGE=ghcr.io/kdlbs/kandev:0.93.0@sha256:1a3c98e62bb3ab20141695b6a6b6fdcd058480dd2ef42a89277724b95afc6884
FROM ${BASE_IMAGE} AS base

USER root

# Keep runtime installs and caches on the Kandev workspace. The executor mounts
# /workspace after image start, so these directories are created in the image
# only to document the expected ownership and to support image smoke tests.
RUN mkdir -p /workspace/.npm-global /workspace/.pnpm /workspace/.pnpm-store /workspace/.cache/pip \
    && chown -R 1000:1000 /workspace

ENV NPM_CONFIG_PREFIX=/workspace/.npm-global \
    PNPM_HOME=/workspace/.pnpm \
    PIP_CACHE_DIR=/workspace/.cache/pip \
    PATH=/workspace/.pnpm:/workspace/.npm-global/bin:$PATH

FROM base AS minimal

USER 1000

FROM base AS node-pnpm

ARG PNPM_VERSION=9.15.9
USER root
RUN npm install --global --prefix /usr/local "pnpm@${PNPM_VERSION}" \
    && pnpm --version

USER 1000

FROM base AS python

USER 1000
