FROM golang:1.26-alpine@sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1 AS builder

# hadolint ignore=DL3018
RUN apk --no-cache add ca-certificates git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /codeowners-validator .

FROM scratch

LABEL org.opencontainers.image.source=https://github.com/step-security/codeowners-validator

COPY --from=builder /codeowners-validator /codeowners-validator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/bin/git /usr/bin/git
COPY --from=builder /usr/bin/xargs /usr/bin/xargs
COPY --from=builder /lib /lib
COPY --from=builder /usr/lib /usr/lib

ENTRYPOINT ["/codeowners-validator"]
