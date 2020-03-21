FROM golang:1.14.1 AS builder
LABEL maintainer="andy.lo-a-foe@philips.com"

WORKDIR /out
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all

FROM fluent/fluent-bit:1.3.11

COPY --from=builder /out/out_hsdp.so /fluent-bit/bin/
COPY *.conf /fluent-bit/etc/

CMD ["/fluent-bit/bin/fluent-bit", "-c", "/fluent-bit/etc/fluent-bit.conf", "-e", "/fluent-bit/bin/out_hsdp.so"]
