FROM golang:1.23 AS builder
WORKDIR /workspace
#ENV GO111MODULE=on CGO_ENABLED=0

COPY . .
RUN go get
RUN go build -o /app/tesla-assistant



FROM golang:1.23 AS release
WORKDIR /app

# Copy from builder
COPY --from=builder /app/tesla-assistant ./tesla-assistant

ENTRYPOINT ["./tesla-assistant"]

