FROM golang:1.21.4 AS builder

WORKDIR /out
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all

FROM busybox:1.36.1
WORKDIR /plugins
COPY --from=builder /out/out_hsdp.so /plugins
