FROM alpine:3.15
WORKDIR /go/src/go-reverse-proxy
COPY . .

CMD ["./go-reverse-proxy", "start"]
