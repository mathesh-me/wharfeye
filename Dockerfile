FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /wharfeye ./cmd/wharfeye

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /wharfeye /usr/local/bin/wharfeye

EXPOSE 9090

ENTRYPOINT ["wharfeye"]
CMD ["web"]
