FROM golang:1.10 AS builder

WORKDIR /go/src/github.com/loafoe/fluent-bit-go-hsdp-output/

COPY .git Makefile Gopkg.* *.go /go/src/github.com/loafoe/fluent-bit-go-hsdp-output/
RUN go get -u github.com/golang/dep/cmd/dep \
 && make dep all

FROM fluent/fluent-bit:0.13.7

COPY --from=builder /go/src/github.com/loafoe/fluent-bit-go-hsdp-output/out_hsdp.so /fluent-bit/bin/
COPY *.conf /fluent-bit/etc/
COPY start.sh /start.sh

CMD ["/start.sh"]
