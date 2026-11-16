# Built natively for linux/arm64 (Fargate Graviton: ~20% cheaper than x86, no emulation on an M-series Mac).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /cart-service .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /cart-service /cart-service
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/cart-service"]
