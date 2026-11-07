# One image, two binaries: /receiver and /processor. Each ECS task definition
# picks its entrypoint. Cross-compiles on the host, runs on linux/amd64 Fargate.
ARG ARCH=amd64

FROM golang:1.27-alpine AS build
ARG ARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -ldflags="-s -w" -o /out/receiver ./cmd/receiver && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -ldflags="-s -w" -o /out/processor ./cmd/processor

FROM --platform=linux/${ARCH} gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/receiver"]
