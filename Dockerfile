FROM golang:alpine AS builder
RUN adduser -D logfwrd
# Copy the code from the host
WORKDIR /
COPY . .
# Compile it
ENV CGO_ENABLED=0
RUN set -xe && \
    apk add upx && \
    go build -ldflags='-s -w -extldflags "-static"' \
    -o /go/bin/logfwrd && \
    upx --lzma /go/bin/logfwrd

# Create the container
FROM alpine
COPY --from=builder /go/bin/logfwrd /usr/bin/logfwrd
COPY --from=0 /etc/passwd /etc/passwd

USER logfwrd
CMD /usr/bin/logfwrd
