# Build stage
FROM golang:1.24-alpine AS builder

ARG SCALER_JOB
RUN echo "SCALER_JOB is $SCALER_JOB"
# Validate build argument SCALER_JOB
RUN if [ -z "$SCALER_JOB" ]; then echo "SCALER_JOB not set"; exit 1; fi

WORKDIR /app
COPY . .

RUN apk add --no-cache git && \
    go env -w GOTOOLCHAIN=local

RUN go mod download

# Build specified component based on SCALER_JOB
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/${SCALER_JOB} ./cmd/${SCALER_JOB}

# Final stage
FROM alpine:latest

ARG SCALER_JOB

# Copy the built binary
COPY --from=builder /bin/${SCALER_JOB} /bin/

# Set environment variable for runtime
ENV SCALER_JOB=${SCALER_JOB}

# Use environment variable to determine which binary to run
ENTRYPOINT ["sh", "-c", "exec $SCALER_JOB"]