FROM ubuntu:22.04
RUN apt update && apt install -y ca-certificates
WORKDIR /app
COPY otpgateway .
COPY config.sample.toml config.toml
COPY static/smtp.tpl ./static/smtp.tpl
CMD ["./otpgateway", "--config", "./config.toml"]
