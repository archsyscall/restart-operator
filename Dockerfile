# Build stage
FROM golang:1.24 as builder

WORKDIR /workspace

# Copy Go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY hack/ hack/

# Build the operator binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the operator binary
COPY --from=builder /workspace/manager .

# Use nonroot user for security
USER 65532:65532

# Expose metrics and health probe ports
EXPOSE 8080 8081

ENTRYPOINT ["/manager"]