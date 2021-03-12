FROM golang:1.16.2 AS builder

WORKDIR /out
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all

FROM fluent/fluent-bit:1.7.2
LABEL maintainer="andy.lo-a-foe@philips.com"

COPY --from=builder /out/out_hsdp.so /fluent-bit/bin/
COPY *.conf /fluent-bit/etc/

CMD ["/fluent-bit/bin/fluent-bit", "-c", "/fluent-bit/etc/fluent-bit.conf", "-e", "/fluent-bit/bin/out_hsdp.so"]
