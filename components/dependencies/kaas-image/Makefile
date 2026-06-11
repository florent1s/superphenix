# =============================================================================
# Builds the KaaS VM disk images.
# =============================================================================
#
# Each file in ./versions/ describes one Kubernetes version we want to build.
# The file's name is the Kubernetes version (e.g. "1.35.5"), and its contents
# are KEY=VALUE pairs that are forwarded to the Dockerfile as `--build-arg`s.
#
# The resulting image is tagged as:
#
#     $(IMAGE):$(SPX_VERSION)-<k8s-version>
#
# where <k8s-version> is the file name in ./versions/.
#
# -----------------------------------------------------------------------------
# Common usage:
#
#   make                                  Build every version (SPX_VERSION=0.0.0)
#   make SPX_VERSION=1.2.3                Build every version, tagged 1.2.3-*
#   make build-1.35.5                     Build a single Kubernetes version
#   make build-1.35.5 SPX_VERSION=1.2.3   Build a single version with a tag
#   make push                             Push every built image
#   make push-1.35.5                      Push a single image
#   make list                             List the version files detected
#
# =============================================================================


# -----------------------------------------------------------------------------
# Configuration (override on the command line, e.g. `make IMAGE=my-image`)
# -----------------------------------------------------------------------------

# Container repository name. Combined with SPX_VERSION and the file name in
# ./versions/ to produce the final tag "$(IMAGE):$(SPX_VERSION)-<k8s-version>".
IMAGE          ?= kaas-image

# Top-level Superphenix version. Becomes the left half of the image tag.
# Defaults to 0.0.0 so that local builds always have a valid tag.
SPX_VERSION    ?= 0.0.0

# Container tool used to build and push images (e.g. docker, podman).
CONTAINER_TOOL ?= docker

# Folder that holds one file per Kubernetes version we want to build.
VERSIONS_DIR   ?= versions


# -----------------------------------------------------------------------------
# Shell setup
# -----------------------------------------------------------------------------

# Use bash with fail-fast behavior so recipes stop on the first error,
# including failures in the middle of a pipeline.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec


# -----------------------------------------------------------------------------
# Version discovery
# -----------------------------------------------------------------------------

# Discover every file in $(VERSIONS_DIR) and keep only its file name. For
# ./versions/1.35.5, VERSIONS contains "1.35.5". This list drives every other
# rule below.
VERSIONS := $(notdir $(wildcard $(VERSIONS_DIR)/*))

# Pre-compute the per-version build and push target names. These are what
# the aggregate `build` and `push` targets depend on.
BUILD_TARGETS := $(addprefix build-,$(VERSIONS))
PUSH_TARGETS  := $(addprefix push-,$(VERSIONS))

# Mark the static aggregate targets as .PHONY so they always run. We
# deliberately do NOT include $(BUILD_TARGETS) / $(PUSH_TARGETS) here:
# the macOS-default GNU Make 3.81 has a bug where declaring a target
# matched by a pattern rule (build-%, push-%) as .PHONY suppresses its
# recipe. We don't need .PHONY for them anyway -- they never produce a
# real file, so make will always re-run the pattern rule.
.PHONY: all list build push


# -----------------------------------------------------------------------------
# Top-level targets
# -----------------------------------------------------------------------------

# Default target: build every version.
all: build

# Print every Kubernetes version detected in $(VERSIONS_DIR)/.
list:
	@for v in $(VERSIONS); do echo "$$v"; done

# Aggregate target: build one image per file in $(VERSIONS_DIR)/.
# This works by depending on every "build-<version>" target.
build: $(BUILD_TARGETS)

# Aggregate target: push every image we just built.
push: $(PUSH_TARGETS)


# -----------------------------------------------------------------------------
# Per-version targets (pattern rules)
# -----------------------------------------------------------------------------
#
# `build-%` and `push-%` are pattern rules. The `%` captures the Kubernetes
# version id (i.e. the file name in $(VERSIONS_DIR)/). Inside the recipe it
# is referenced as `$*`. So for `make build-1.35.5`:
#
#     $*                        == "1.35.5"
#     $(VERSIONS_DIR)/$*        == "versions/1.35.5"
#     $(IMAGE):$(SPX_VERSION)-$* == "kaas-image:0.0.0-1.35.5"
#
# The build recipe must run as a single shell invocation (one long block
# glued together with `\`), because it accumulates `--build-arg` flags
# across multiple iterations of a while-loop into the `build_args` variable.
# Comments cannot live inside that block (a shell `#` would swallow the
# trailing `\` and break the continuation), so they are placed above instead:
#
#   1. `test -f` makes sure the version file exists, so typos fail loudly
#      instead of silently building with the Dockerfile's default ARGs.
#   2. The while-loop reads the version file line by line. Each line is
#      split on the FIRST `=` into a key and a value. Blank lines and
#      lines starting with `#` are skipped. Whitespace around the key is
#      trimmed. The remaining KEY=VALUE becomes a `--build-arg` flag.
#   3. Finally, `docker build` is invoked with all accumulated build args
#      and tagged as $(IMAGE):$(SPX_VERSION)-<version>.

build-%:
	@test -f "$(VERSIONS_DIR)/$*" \
		|| { echo "Error: $(VERSIONS_DIR)/$* not found"; exit 1; }
	@echo "==> Building $(IMAGE):$(SPX_VERSION)-$* from $(VERSIONS_DIR)/$*"
	@build_args=""; \
	while IFS='=' read -r key value || [ -n "$$key" ]; do \
		case "$$key" in ''|'#'*) continue ;; esac; \
		key="$$(echo $$key | tr -d '[:space:]')"; \
		[ -z "$$key" ] && continue; \
		build_args="$$build_args --build-arg $$key=$$value"; \
	done < "$(VERSIONS_DIR)/$*"; \
	$(CONTAINER_TOOL) build $$build_args \
		-t "$(IMAGE):$(SPX_VERSION)-$*" \
		-f Dockerfile .

# Push the image that `build-<version>` produced. The tag is rebuilt from
# $(IMAGE), $(SPX_VERSION) and the captured `$*` so that `make push-1.35.5`
# and `make build-1.35.5` always agree on the tag.
push-%:
	$(CONTAINER_TOOL) push "$(IMAGE):$(SPX_VERSION)-$*"
