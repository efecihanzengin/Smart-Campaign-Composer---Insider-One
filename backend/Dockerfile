FROM golang:1.25-alpine AS base
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Used by `make test` when Go is not installed on the host.
FROM base AS test
CMD ["go", "test", "-count=1", "./..."]

FROM base AS build
RUN go build -o /out/campaign ./cmd/campaign && go build -o /out/provider ./cmd/provider

FROM alpine:3.22
COPY --from=build /out/ /usr/local/bin/
