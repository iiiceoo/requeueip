ARG BUILDER_IMAGE
ARG BASE_IMAGE

FROM --platform=${BUILDPLATFORM} ${BUILDER_IMAGE} as builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy the go source.
COPY . .

# Build binary.
RUN make go.build.${TARGETOS}_${TARGETARCH}.requeueip-controller

FROM --platform=${BUILDPLATFORM} ${BASE_IMAGE}

ARG TARGETOS
ARG TARGETARCH

WORKDIR /

# Copy the binary.
COPY --from=builder /workspace/_output/platforms/${TARGETOS}/${TARGETARCH}/requeueip-controller /usr/bin

ENTRYPOINT ["requeueip-controller"]