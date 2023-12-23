FROM golang:alpine as builder

# Copy the code from the host
RUN adduser -D logfwrd
WORKDIR /
COPY . .

# Compile it
RUN CGO_ENABLED=0 GOOS=linux go build -a \
  -installsuffix cgo \
  -ldflags '-extldflags "-static"' \
  -o logfwrd .

# Create the container
FROM scratch
COPY --from=builder /logfwrd /app/logfwrd
COPY --from=0 /etc/passwd /etc/passwd
USER logfwrd

ENTRYPOINT ["/app/logfwrd"]
