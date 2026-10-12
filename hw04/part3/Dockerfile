FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG SERVICE TARGETOS TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12
COPY --from=build /service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
