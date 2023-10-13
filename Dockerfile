FROM golang:1.20.6-alpine3.18 as builder

COPY . /build
WORKDIR /build
RUN go build -o s4

FROM alpine:3.18

COPY --from=builder /build/s4 /usr/local/s4/s4
COPY --from=builder /build/static /usr/local/s4/static

ENV PATH="/usr/local/s4:${PATH}"

WORKDIR /usr/local/s4

CMD ["/usr/local/s4/s4", "--address", "0.0.0.0:8090", "--asset-dir", "static"]
