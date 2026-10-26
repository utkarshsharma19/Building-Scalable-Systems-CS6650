ARG ARCH=amd64

FROM golang:1.27-alpine AS build
ARG ARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -ldflags="-s -w" -o /product-service .

FROM --platform=linux/${ARCH} gcr.io/distroless/static-debian12:nonroot
COPY --from=build /product-service /product-service
ENV GIN_MODE=release PORT=8080
EXPOSE 8080
ENTRYPOINT ["/product-service"]
