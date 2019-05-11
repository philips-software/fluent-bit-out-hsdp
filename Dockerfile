FROM golang:1.12.5 AS builder
LABEL maintainer="andy.lo-a-foe@philips.com"

WORKDIR /out
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all

FROM fluent/fluent-bit:0.13.7

COPY --from=builder /out/out_hsdp.so /fluent-bit/bin/
COPY *.conf /fluent-bit/etc/
COPY start.sh /start.sh

CMD ["/start.sh"]
