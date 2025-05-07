FROM golang:1.24.3 AS builder

WORKDIR /out
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN make all

FROM busybox:1.37.0
WORKDIR /plugins
COPY --from=builder /out/out_hsdp.so /plugins
