FROM golang:1.13 as gobuilder

WORKDIR /root

ENV GOOS=linux\
    GOARCH=amd64

COPY / /root/

RUN go build \
    -buildmode=c-shared \
    -o /out_gostackdriver.so \
    github.com/urtho/fluent-bit-out-gostackdriver

FROM fluent/fluent-bit:1.3.5

COPY --from=gobuilder /out_gostackdriver.so /fluent-bit/bin/

EXPOSE 2020

# CMD ["/fluent-bit/bin/fluent-bit", "--plugin", "/fluent-bit/bin/out_gostackdriver.so", "--config", "/fluent-bit/etc/fluent-bit.conf"]¬
CMD ["/fluent-bit/bin/fluent-bit", "--config", "/fluent-bit/etc/fluent-bit.conf"]¬
