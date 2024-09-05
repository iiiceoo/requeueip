ARG BUILDER_IMAGE
ARG BASE_IMAGE

FROM --platform=${BUILDPLATFORM} ${BUILDER_IMAGE} as builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Copy the go source.
COPY . .

# Build binary.
RUN make go.build.${TARGETOS}_${TARGETARCH}.requeueip
RUN make go.build.${TARGETOS}_${TARGETARCH}.requeueipd

FROM ${BASE_IMAGE}

ARG TARGETOS
ARG TARGETARCH

WORKDIR /

# Copy the binary.
COPY --from=builder /workspace/_output/platforms/${TARGETOS}/${TARGETARCH}/requeueip /usr/bin
COPY --from=builder /workspace/_output/platforms/${TARGETOS}/${TARGETARCH}/requeueipd /usr/bin

ENTRYPOINT ["requeueipd"]