FROM golang:1.27 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /albums-server

FROM gcr.io/distroless/static-debian12
COPY --from=build /albums-server /albums-server
EXPOSE 8080
ENTRYPOINT ["/albums-server"]
