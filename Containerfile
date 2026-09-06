# FROM --platform=${TARGETPLATFORM:-linux/amd64} scratch
# FROM --platform=${TARGETPLATFORM:-linux/amd64} alpine:latest

# https://github.com/GoogleContainerTools/distroless/blob/main/base/README.md
#
# The :nonroot variant, and USER below: the image used to run as root, which
# turns any code execution in the service into root in the container. Pinned
# to the debian12 line and the nonroot tag; a digest pin is per-architecture
# on this image and the build is multi-arch, so the tag is the pin.
FROM --platform=${TARGETPLATFORM:-linux/amd64} gcr.io/distroless/base-debian12:nonroot

# these parameters are required
# example: --build-arg SERVICE_NAME=go-rest-api-service-template --build-arg GOOS=linux --build-arg GOARCH=arm64
ARG SERVICE_NAME
ARG BUILD_DATE
ARG BUILD_VERSION
ARG DESCRIPTION
ARG REPO_URL
ARG GOOS
ARG GOARCH

# https://github.com/opencontainers/image-spec/blob/main/annotations.md
LABEL org.opencontainers.image.created=${BUILD_DATE}
LABEL org.opencontainers.image.title=${SERVICE_NAME}
LABEL org.opencontainers.image.version=$BUILD_VERSION
LABEL org.opencontainers.image.description=${DESCRIPTION}
LABEL org.opencontainers.image.source=${REPO_URL}

# make available the service name in the container
ENV SERVICE_NAME=${SERVICE_NAME}

WORKDIR /app
ENV PATH="/app:${PATH}"

COPY --chmod=755 "dist/${SERVICE_NAME}-${GOOS}-${GOARCH}" /app/microservice

USER nonroot:nonroot

ENTRYPOINT ["/app/microservice"]
