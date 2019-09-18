FROM golang:1.12-alpine AS builder
RUN apk update && apk add gcc libc-dev make git
WORKDIR /otpgateway/
COPY ./ ./
ENV CGO_ENABLED=1 GOOS=linux
RUN make deps
RUN make build

FROM alpine:latest AS deploy
RUN apk --no-cache add ca-certificates
WORKDIR /otpgateway/
COPY --from=builder /otpgateway/static/ static/
COPY --from=builder /otpgateway/otpgateway /otpgateway/config.toml.sample /otpgateway/smtp.prov /otpgateway/solsms.prov /otpgateway/pinpoint.prov ./
RUN mkdir -p /etc/otpgateway && cp config.toml.sample /etc/otpgateway/config.toml
VOLUME ["/etc/otpgateway"]
CMD ["./otpgateway", "--config", "/etc/otpgateway/config.toml"]
