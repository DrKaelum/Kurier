FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/viability-service .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates
COPY --from=build /out/viability-service /usr/local/bin/viability-service
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --retries=3 \
  CMD wget --quiet --spider http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/viability-service"]
