FROM golang:1.20-alpine as builder
COPY . /tmp/myService
ENV GO111MODULE=on
ENV GOPROXY="https://goproxy.cn"
WORKDIR /tmp/myService
RUN --mount=type=cache,target=/root/.cache/go-build go build -o mailserver main.go

FROM alpine
WORKDIR /usr/local/bin

EXPOSE 8080

COPY --from=builder /tmp/myService/mailserver /usr/local/bin/
COPY --from=builder /tmp/myService/config.yaml /usr/local/bin/

ENTRYPOINT ["mailserver"]