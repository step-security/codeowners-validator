FROM golang:1.21-alpine AS builder

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
